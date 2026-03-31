package handlers

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"os"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/social"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

const multipartUploadOverheadBytes = 1 << 20

var (
	errUploadedFileTooLarge = errors.New("uploaded file too large")
	errMissingUploadedFile  = errors.New("missing file upload")
	errEmptyUploadedFile    = errors.New("empty file")
)

type stagedUpload struct {
	Path     string
	Filename string
	Size     int64
	Sniff    []byte
}

type contentSource struct {
	io.ReadSeeker
	size    int64
	closeFn func() error
}

func (s *contentSource) Close() error {
	if s == nil || s.closeFn == nil {
		return nil
	}
	return s.closeFn()
}

// UploadMedia handles POST /api/media (multipart file upload).
func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	maxUploadBytes := int64(content.MaxMediaSize)
	if h.cfg != nil {
		if configured := h.cfg.MaxUploadBytes(); configured > 0 && configured < maxUploadBytes {
			maxUploadBytes = configured
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+multipartUploadOverheadBytes)

	upload, err := stageUploadedFile(r, maxUploadBytes)
	if err != nil {
		switch {
		case errors.Is(err, errUploadedFileTooLarge):
			respondError(w, http.StatusRequestEntityTooLarge, errUploadedFileTooLarge.Error())
		case errors.Is(err, errMissingUploadedFile), errors.Is(err, errEmptyUploadedFile):
			respondError(w, http.StatusBadRequest, err.Error())
		default:
			respondBadRequest(w, "failed to stage uploaded file", err, "failed to read uploaded file")
		}
		return
	}
	defer os.Remove(upload.Path)

	// Detect mime type from the initial bytes without buffering the full upload.
	mimeType := http.DetectContentType(upload.Sniff)
	if err := content.ValidateMediaType(mimeType); err != nil {
		respondBadRequest(w, "rejected uploaded media type", err, "unsupported media type", "mime_type", mimeType)
		return
	}

	// Extract and validate metadata before any expensive image decoding.
	var width, height, duration uint32
	if isImageMime(mimeType) {
		metaReader, err := os.Open(upload.Path)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to inspect uploaded media")
			return
		}
		meta, err := content.ExtractImageMetadataReader(metaReader)
		metaReader.Close()
		if err != nil {
			respondBadRequest(w, "rejected invalid uploaded image", err, "invalid image", "mime_type", mimeType)
			return
		}
		width = meta.Width
		height = meta.Height
	}

	// Stream chunk storage from the staged file so large uploads do not remain
	// resident in memory after multipart parsing.
	chunks, err := h.storeMediaChunksFromFile(upload.Path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store media chunks")
		return
	}

	// Compute the full-file CID from the staged upload stream.
	fileCID, err := computeFileCID(upload.Path)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to compute file CID")
		return
	}

	// Store the full file stream in CAS without reloading it into memory.
	if err := h.storeMediaFileFromPath(fileCID, upload.Path); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store media file")
		return
	}

	thumbnailQuality := content.ThumbnailJPEGQuality
	if h.cfg != nil {
		thumbnailQuality = h.cfg.ThumbnailJPEGQuality()
	}

	// Try to generate a thumbnail for images and videos.
	var thumbnailCID []byte
	if isImageMime(mimeType) || isVideoMime(mimeType) {
		thumbData, thumbCID, err := generateMediaThumbnailFromPath(upload.Path, mimeType, thumbnailQuality)
		if err == nil {
			if err := h.cas.Put(thumbCID, thumbData); err != nil {
				slog.Error("failed to store thumbnail in CAS", "error", err)
			} else {
				thumbnailCID = thumbCID
			}
		}
	}

	// Store media metadata in DB.
	timestamp := time.Now().UnixMilli()
	if err := h.db.InsertMediaObject(
		fileCID,
		kp.PublicKeyBytes(),
		mimeType,
		uint64(upload.Size),
		uint32(len(chunks)),
		width, height, duration,
		thumbnailCID,
		timestamp,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store media metadata")
		return
	}

	// Mark as fully fetched since we have all chunks locally.
	if err := h.db.SetMediaFetched(fileCID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update media fetch state")
		return
	}
	if err := h.db.TrackContentForMedia(fileCID, fileCID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to track media content")
		return
	}
	if len(thumbnailCID) > 0 {
		if err := h.db.TrackContentForMedia(thumbnailCID, fileCID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to track media thumbnail")
			return
		}
	}
	for _, chunk := range chunks {
		if err := h.db.TrackContentForMedia(chunk.CID, fileCID); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to track media chunk")
			return
		}
	}

	mediaObj, err := signMediaObject(kp, fileCID, mimeType, chunks, thumbnailCID, width, height, duration, timestamp, uint64(upload.Size))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to sign media metadata")
		return
	}
	if h.p2pHost != nil {
		if err := social.PublishMediaObject(r.Context(), h.p2pHost, mediaObj); err != nil {
			slog.Error("failed to publish media metadata", "error", err)
		}
		h.provideContent(r.Context(), fileCID, thumbnailCID, chunks)
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"cid":           hex.EncodeToString(fileCID),
		"mime_type":     mimeType,
		"size":          upload.Size,
		"chunk_count":   len(chunks),
		"width":         width,
		"height":        height,
		"duration":      duration,
		"thumbnail_cid": hexOrEmpty(thumbnailCID),
		"filename":      upload.Filename,
	})
}

func stageUploadedFile(r *http.Request, maxUploadBytes int64) (*stagedUpload, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		if isRequestTooLarge(err) {
			return nil, errUploadedFileTooLarge
		}
		return nil, err
	}

	var upload *stagedUpload
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if isRequestTooLarge(err) {
				return nil, errUploadedFileTooLarge
			}
			return nil, err
		}

		if part.FormName() != "file" || part.FileName() == "" {
			if err := discardMultipartPart(part); err != nil {
				part.Close()
				if isRequestTooLarge(err) {
					return nil, errUploadedFileTooLarge
				}
				return nil, err
			}
			part.Close()
			continue
		}

		if upload != nil {
			part.Close()
			return nil, fmt.Errorf("multiple file uploads are not supported")
		}

		upload, err = copyMultipartPartToTempFile(part, maxUploadBytes)
		part.Close()
		if err != nil {
			return nil, err
		}
	}

	if upload == nil {
		return nil, errMissingUploadedFile
	}
	if upload.Size == 0 {
		return nil, errEmptyUploadedFile
	}

	return upload, nil
}

func copyMultipartPartToTempFile(part *multipart.Part, maxUploadBytes int64) (*stagedUpload, error) {
	temp, err := os.CreateTemp("", "xleaks-upload-*")
	if err != nil {
		return nil, fmt.Errorf("create temp upload file: %w", err)
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			temp.Close()
			_ = os.Remove(tempPath)
		}
	}()

	sniff := make([]byte, 0, 512)
	buf := make([]byte, 32*1024)
	var size int64

	for {
		n, err := part.Read(buf)
		if n > 0 {
			size += int64(n)
			if size > maxUploadBytes {
				return nil, errUploadedFileTooLarge
			}
			if len(sniff) < cap(sniff) {
				sniffCount := n
				if remaining := cap(sniff) - len(sniff); sniffCount > remaining {
					sniffCount = remaining
				}
				sniff = append(sniff, buf[:sniffCount]...)
			}
			if _, writeErr := temp.Write(buf[:n]); writeErr != nil {
				return nil, fmt.Errorf("write temp upload file: %w", writeErr)
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			if isRequestTooLarge(err) {
				return nil, errUploadedFileTooLarge
			}
			return nil, fmt.Errorf("read uploaded file: %w", err)
		}
	}

	if err := temp.Sync(); err != nil {
		return nil, fmt.Errorf("sync temp upload file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return nil, fmt.Errorf("close temp upload file: %w", err)
	}

	cleanup = false
	return &stagedUpload{
		Path:     tempPath,
		Filename: part.FileName(),
		Size:     size,
		Sniff:    sniff,
	}, nil
}

func discardMultipartPart(part *multipart.Part) error {
	_, err := io.Copy(io.Discard, part)
	return err
}

func isRequestTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func (h *Handler) storeMediaChunksFromFile(path string) ([]content.Chunk, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var chunks []content.Chunk
	buf := make([]byte, content.ChunkSize)
	index := 0
	for {
		n, err := io.ReadFull(file, buf)
		if n > 0 {
			chunkData := append([]byte(nil), buf[:n]...)
			cid, cidErr := content.ComputeCID(chunkData)
			if cidErr != nil {
				return nil, cidErr
			}
			if putErr := h.cas.Put(cid, chunkData); putErr != nil {
				return nil, putErr
			}
			chunks = append(chunks, content.Chunk{CID: cid, Index: index})
			index++
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return chunks, nil
}

func computeFileCID(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return content.ComputeCIDReader(file)
}

func (h *Handler) storeMediaFileFromPath(cid []byte, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return h.cas.PutReader(cid, file)
}

func generateMediaThumbnailFromPath(path, mimeType string, quality int) ([]byte, []byte, error) {
	if isVideoMime(mimeType) {
		return content.GenerateMediaThumbnailReader(nil, mimeType, quality)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer file.Close()
	return content.GenerateMediaThumbnailReader(file, mimeType, quality)
}

// GetMedia handles GET /api/media/{cid}.
func (h *Handler) GetMedia(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	media, _ := h.db.GetMediaObject(cidBytes)
	source, err := h.openContent(r.Context(), cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "media data not available")
		return
	}
	defer source.Close()
	if media != nil {
		if err := h.db.TrackContentForMedia(cidBytes, cidBytes); err != nil {
			slog.Warn("failed to track media access", "cid", hex.EncodeToString(cidBytes), "error", err)
		}
	}

	mimeType := ""
	if media != nil {
		mimeType = media.MimeType
	}
	mimeType, err = detectContentType(source, mimeType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read media data")
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeContent(w, r, "", time.Time{}, source)
}

// GetMediaThumbnail handles GET /api/media/{cid}/thumbnail.
func (h *Handler) GetMediaThumbnail(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	media, _ := h.db.GetMediaObject(cidBytes)
	if media != nil && len(media.ThumbnailCID) > 0 {
		source, err := h.openContent(r.Context(), media.ThumbnailCID)
		if err == nil {
			defer source.Close()
			if err := h.db.TrackContentForMedia(media.ThumbnailCID, media.CID); err != nil {
				slog.Warn("failed to track thumbnail access", "cid", hex.EncodeToString(media.ThumbnailCID), "error", err)
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			http.ServeContent(w, r, "", time.Time{}, source)
			return
		}
	}

	source, err := h.openContent(r.Context(), cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "thumbnail data not available")
		return
	}
	defer source.Close()

	mimeType := ""
	if media != nil {
		mimeType = media.MimeType
	}
	mimeType, err = detectContentType(source, mimeType)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "thumbnail data not available")
		return
	}
	if !isImageMime(mimeType) && !isVideoMime(mimeType) {
		respondError(w, http.StatusNotFound, "no thumbnail available")
		return
	}
	thumbnailQuality := content.ThumbnailJPEGQuality
	if h.cfg != nil {
		thumbnailQuality = h.cfg.ThumbnailJPEGQuality()
	}
	thumbData, _, err := content.GenerateMediaThumbnailReader(source, mimeType, thumbnailQuality)
	if err != nil {
		respondError(w, http.StatusNotFound, "no thumbnail available")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	w.Write(thumbData)
}

// GetMediaStatus handles GET /api/media/{cid}/status.
func (h *Handler) GetMediaStatus(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	media, _ := h.db.GetMediaObject(cidBytes)
	if media != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"cid":           hex.EncodeToString(media.CID),
			"mime_type":     media.MimeType,
			"size":          media.Size,
			"chunk_count":   media.ChunkCount,
			"width":         media.Width,
			"height":        media.Height,
			"duration":      media.Duration,
			"thumbnail_cid": hexOrEmpty(media.ThumbnailCID),
			"fully_fetched": media.FullyFetched,
		})
		return
	}

	source, err := h.openContent(r.Context(), cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "media not found")
		return
	}
	defer source.Close()

	mimeType, err := detectContentType(source, "")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "media not found")
		return
	}

	var width, height, duration uint32
	if isImageMime(mimeType) {
		meta, metaErr := content.ExtractImageMetadataReader(source)
		if metaErr == nil {
			width = meta.Width
			height = meta.Height
		}
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"cid":           hex.EncodeToString(cidBytes),
		"mime_type":     mimeType,
		"size":          source.size,
		"chunk_count":   1,
		"width":         width,
		"height":        height,
		"duration":      duration,
		"thumbnail_cid": "",
		"fully_fetched": true,
	})
}

// isImageMime returns true if the MIME type indicates an image.
func isImageMime(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

func isVideoMime(mime string) bool {
	switch mime {
	case "video/mp4", "video/webm":
		return true
	}
	return false
}

func signMediaObject(kp *identity.KeyPair, cid []byte, mimeType string, chunks []content.Chunk, thumbnailCID []byte, width, height, duration uint32, timestamp int64, size uint64) (*pb.MediaObject, error) {
	chunkCIDs := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		chunkCIDs = append(chunkCIDs, chunk.CID)
	}
	obj := &pb.MediaObject{
		Cid:          cid,
		Author:       kp.PublicKeyBytes(),
		MimeType:     mimeType,
		Size:         size,
		ChunkCount:   uint32(len(chunks)),
		ChunkCids:    chunkCIDs,
		Width:        width,
		Height:       height,
		Duration:     duration,
		ThumbnailCid: thumbnailCID,
		Timestamp:    uint64(timestamp),
	}

	clone := proto.Clone(obj).(*pb.MediaObject)
	clone.Signature = nil
	payload, err := proto.Marshal(clone)
	if err != nil {
		return nil, err
	}
	sig, err := identity.SignProtoMessage(kp, payload)
	if err != nil {
		return nil, err
	}
	obj.Signature = sig
	if err := content.ValidateMediaObject(obj, func(pubkey, message, signature []byte) bool {
		return identity.Verify(pubkey, message, signature)
	}); err != nil {
		return nil, err
	}
	return obj, nil
}

func (h *Handler) openContent(ctx context.Context, cidBytes []byte) (*contentSource, error) {
	source, err := h.openStoredContent(cidBytes)
	if err == nil {
		h.trackContentAccess(cidBytes)
		return source, nil
	}
	if h.p2pHost == nil {
		return nil, err
	}
	ce := h.p2pHost.ContentExchange()
	if ce == nil {
		return nil, err
	}
	cidHex := hex.EncodeToString(cidBytes)
	fetched, fetchErr := ce.FetchContentToTempFile(ctx, cidHex)
	if fetchErr != nil {
		return nil, fetchErr
	}
	if putErr := h.cacheFetchedContent(cidBytes, fetched.Path); putErr == nil {
		source, openErr := h.openStoredContent(cidBytes)
		if openErr == nil {
			if removeErr := os.Remove(fetched.Path); removeErr != nil && !os.IsNotExist(removeErr) {
				slog.Warn("failed to remove fetched temp media", "cid", cidHex, "error", removeErr)
			}
			h.trackContentAccess(cidBytes)
			return source, nil
		}
	} else {
		slog.Warn("failed to cache fetched media", "cid", cidHex, "error", putErr)
	}

	h.trackContentAccess(cidBytes)
	file, openErr := os.Open(fetched.Path)
	if openErr != nil {
		if removeErr := os.Remove(fetched.Path); removeErr != nil && !os.IsNotExist(removeErr) {
			slog.Warn("failed to remove fetched temp media after open error", "cid", cidHex, "error", removeErr)
		}
		return nil, openErr
	}
	return &contentSource{
		ReadSeeker: file,
		size:       fetched.Size,
		closeFn: func() error {
			closeErr := file.Close()
			removeErr := os.Remove(fetched.Path)
			if closeErr != nil {
				return closeErr
			}
			if removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
			return nil
		},
	}, nil
}

func (h *Handler) openStoredContent(cidBytes []byte) (*contentSource, error) {
	file, err := h.cas.Open(cidBytes)
	if err != nil {
		return nil, err
	}
	info, statErr := file.Stat()
	if statErr != nil {
		file.Close()
		return nil, statErr
	}
	return &contentSource{
		ReadSeeker: file,
		size:       info.Size(),
		closeFn:    file.Close,
	}, nil
}

func (h *Handler) cacheFetchedContent(cidBytes []byte, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return h.cas.PutReader(cidBytes, file)
}

func (h *Handler) trackContentAccess(cidBytes []byte) {
	if trackErr := h.db.TrackContentAccess(cidBytes, false); trackErr != nil {
		slog.Warn("failed to track content access", "cid", hex.EncodeToString(cidBytes), "error", trackErr)
	}
}

func detectContentType(source *contentSource, fallback string) (string, error) {
	if fallback != "" {
		if _, err := source.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		return fallback, nil
	}

	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	var sniff [512]byte
	n, err := source.Read(sniff[:])
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	return http.DetectContentType(sniff[:n]), nil
}

func (h *Handler) provideContent(_ context.Context, fileCID, thumbnailCID []byte, chunks []content.Chunk) {
	if h.p2pHost == nil {
		return
	}
	ce := h.p2pHost.ContentExchange()
	if ce == nil {
		return
	}

	go func() {
		provideCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		cids := make([][]byte, 0, len(chunks)+2)
		cids = append(cids, fileCID)
		if len(thumbnailCID) > 0 {
			cids = append(cids, thumbnailCID)
		}
		for _, chunk := range chunks {
			cids = append(cids, chunk.CID)
		}

		for _, cid := range cids {
			if len(cid) == 0 {
				continue
			}
			if err := ce.Provide(provideCtx, hex.EncodeToString(cid)); err != nil {
				slog.Warn("failed to provide content", "cid", hex.EncodeToString(cid), "error", err)
			}
		}
	}()
}

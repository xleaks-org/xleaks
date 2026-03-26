package handlers

import (
	"context"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
	"github.com/xleaks-org/xleaks/pkg/identity"
	"github.com/xleaks-org/xleaks/pkg/social"
	pb "github.com/xleaks-org/xleaks/proto/gen"
	"google.golang.org/protobuf/proto"
)

// UploadMedia handles POST /api/media (multipart file upload).
func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	kp, ok := h.requireIdentity(w)
	if !ok {
		return
	}

	// Limit upload size to MaxMediaSize.
	r.Body = http.MaxBytesReader(w, r.Body, content.MaxMediaSize)

	file, header, err := r.FormFile("file")
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read uploaded file: "+err.Error())
		return
	}
	defer file.Close()

	// Read the entire file into memory.
	data, err := io.ReadAll(file)
	if err != nil {
		respondError(w, http.StatusBadRequest, "failed to read file data: "+err.Error())
		return
	}

	if len(data) == 0 {
		respondError(w, http.StatusBadRequest, "empty file")
		return
	}

	// Chunk the file.
	chunks, err := content.ChunkFile(data)
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Compute a CID for the full file data.
	fileCID, err := content.ComputeCID(data)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to compute file CID")
		return
	}

	// Store each chunk in CAS.
	for _, chunk := range chunks {
		if err := h.cas.Put(chunk.CID, chunk.Data); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to store chunk")
			return
		}
	}

	// Store the full file data in CAS as well.
	if err := h.cas.Put(fileCID, data); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store media file")
		return
	}

	// Detect mime type.
	mimeType := http.DetectContentType(data)
	if err := content.ValidateMediaType(mimeType); err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Extract media metadata (width, height, duration).
	var width, height, duration uint32
	if meta := content.ExtractMediaMetadata(data, mimeType); meta != nil {
		width = meta.Width
		height = meta.Height
		duration = meta.Duration
	}

	// Try to generate a thumbnail for images.
	var thumbnailCID []byte
	if isImageMime(mimeType) {
		thumbData, thumbCID, err := content.GenerateThumbnail(data)
		if err == nil {
			h.cas.Put(thumbCID, thumbData)
			thumbnailCID = thumbCID
		}
	}

	// Store media metadata in DB.
	timestamp := time.Now().UnixMilli()
	if err := h.db.InsertMediaObject(
		fileCID,
		kp.PublicKeyBytes(),
		mimeType,
		uint64(len(data)),
		uint32(len(chunks)),
		width, height, duration,
		thumbnailCID,
		timestamp,
	); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to store media metadata")
		return
	}

	// Mark as fully fetched since we have all chunks locally.
	h.db.SetMediaFetched(fileCID)

	mediaObj, err := signMediaObject(kp, fileCID, mimeType, chunks, thumbnailCID, width, height, duration, timestamp, uint64(len(data)))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to sign media metadata")
		return
	}
	if h.p2pHost != nil {
		if err := social.PublishMediaObject(r.Context(), h.p2pHost, mediaObj); err != nil {
			log.Printf("publish media metadata: %v", err)
		}
		h.provideContent(r.Context(), fileCID, thumbnailCID, chunks)
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"cid":           hex.EncodeToString(fileCID),
		"mime_type":     mimeType,
		"size":          len(data),
		"chunk_count":   len(chunks),
		"width":         width,
		"height":        height,
		"duration":      duration,
		"thumbnail_cid": hexOrEmpty(thumbnailCID),
		"filename":      header.Filename,
	})
}

// GetMedia handles GET /api/media/{cid}.
func (h *Handler) GetMedia(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	media, _ := h.db.GetMediaObject(cidBytes)
	data, err := h.loadContent(r.Context(), cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "media data not available")
		return
	}

	mimeType := http.DetectContentType(data)
	if media != nil && media.MimeType != "" {
		mimeType = media.MimeType
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
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
		data, err := h.loadContent(r.Context(), media.ThumbnailCID)
		if err == nil {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.WriteHeader(http.StatusOK)
			w.Write(data)
			return
		}
	}

	raw, err := h.loadContent(r.Context(), cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "thumbnail data not available")
		return
	}
	if !isImageMime(http.DetectContentType(raw)) {
		respondError(w, http.StatusNotFound, "no thumbnail available")
		return
	}
	thumbData, _, err := content.GenerateThumbnail(raw)
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

	data, err := h.loadContent(r.Context(), cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "media not found")
		return
	}
	mimeType := http.DetectContentType(data)
	var width, height, duration uint32
	if meta := content.ExtractMediaMetadata(data, mimeType); meta != nil {
		width = meta.Width
		height = meta.Height
		duration = meta.Duration
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"cid":           hex.EncodeToString(cidBytes),
		"mime_type":     mimeType,
		"size":          len(data),
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

func (h *Handler) loadContent(ctx context.Context, cidBytes []byte) ([]byte, error) {
	data, err := h.cas.Get(cidBytes)
	if err == nil {
		return data, nil
	}
	if h.p2pHost == nil {
		return nil, err
	}
	ce := h.p2pHost.ContentExchange()
	if ce == nil {
		return nil, err
	}
	cidHex := hex.EncodeToString(cidBytes)
	data, err = ce.FetchContent(ctx, cidHex, func(cidHex string) ([]byte, error) {
		localCID, convErr := content.HexToCID(cidHex)
		if convErr != nil {
			return nil, convErr
		}
		return h.cas.Get(localCID)
	})
	if err != nil {
		return nil, err
	}
	if putErr := h.cas.Put(cidBytes, data); putErr != nil {
		log.Printf("cache fetched media %s: %v", cidHex, putErr)
	}
	return data, nil
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
				log.Printf("provide content %x: %v", cid, err)
			}
		}
	}()
}

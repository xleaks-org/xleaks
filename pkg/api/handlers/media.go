package handlers

import (
	"encoding/hex"
	"io"
	"net/http"
	"time"

	"github.com/xleaks-org/xleaks/pkg/content"
)

// UploadMedia handles POST /api/media (multipart file upload).
func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
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
		h.kp.PublicKeyBytes(),
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

	// Look up media metadata.
	media, err := h.db.GetMediaObject(cidBytes)
	if err != nil || media == nil {
		respondError(w, http.StatusNotFound, "media not found")
		return
	}

	// Retrieve the full file from CAS.
	data, err := h.cas.Get(cidBytes)
	if err != nil {
		respondError(w, http.StatusNotFound, "media data not available")
		return
	}

	w.Header().Set("Content-Type", media.MimeType)
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

	// Look up media metadata to find thumbnail CID.
	media, err := h.db.GetMediaObject(cidBytes)
	if err != nil || media == nil {
		respondError(w, http.StatusNotFound, "media not found")
		return
	}

	if len(media.ThumbnailCID) == 0 {
		respondError(w, http.StatusNotFound, "no thumbnail available")
		return
	}

	// Retrieve thumbnail from CAS.
	data, err := h.cas.Get(media.ThumbnailCID)
	if err != nil {
		respondError(w, http.StatusNotFound, "thumbnail data not available")
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// GetMediaStatus handles GET /api/media/{cid}/status.
func (h *Handler) GetMediaStatus(w http.ResponseWriter, r *http.Request) {
	cidBytes, err := parseHexParam(r, "cid")
	if err != nil {
		respondError(w, http.StatusBadRequest, err.Error())
		return
	}

	media, err := h.db.GetMediaObject(cidBytes)
	if err != nil || media == nil {
		respondError(w, http.StatusNotFound, "media not found")
		return
	}

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
}

// isImageMime returns true if the MIME type indicates an image.
func isImageMime(mime string) bool {
	switch mime {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

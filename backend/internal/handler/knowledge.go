package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/service"
)

const knowledgeMaxUploadBytes = 32 << 20

// KnowledgeHandler exposes HTTP endpoints for the config-backed knowledge base.
type KnowledgeHandler struct {
	svc *service.KnowledgeService
}

func NewKnowledgeHandler(svc *service.KnowledgeService) *KnowledgeHandler {
	return &KnowledgeHandler{svc: svc}
}

type connectionURIRequest struct {
	ConnectionURI string `json:"connectionUri"`
}

func (h *KnowledgeHandler) GetConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := h.svc.GetConnection(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func (h *KnowledgeHandler) UpdateConnection() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req connectionURIRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.ConnectionURI == "" {
			writeError(w, http.StatusBadRequest, "connectionUri is required")
			return
		}

		conn, err := h.svc.SetConnection(r.Context(), req.ConnectionURI)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func (h *KnowledgeHandler) UploadDocument() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(knowledgeMaxUploadBytes); err != nil {
			writeError(w, http.StatusBadRequest, "invalid multipart form")
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "file is required")
			return
		}
		defer file.Close()

		content, err := service.ReadUploadContent(file, knowledgeMaxUploadBytes)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		collection := r.FormValue("collection")

		result, err := h.svc.UploadDocument(r.Context(), collection, header.Filename, content)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *KnowledgeHandler) GetStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collection := r.URL.Query().Get("collection")
		status, err := h.svc.Status(r.Context(), collection)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, status)
	}
}

func (h *KnowledgeHandler) ListCollections() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		collections, err := h.svc.ListCollections(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, collections)
	}
}

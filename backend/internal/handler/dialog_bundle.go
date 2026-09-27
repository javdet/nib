package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"unicode"

	"github.com/javdet/nib/internal/service"
)

// planBundleMaxBytes caps an imported .nib file. It is well above any plan's
// text; what fills a file is attachments, each up to 16 MiB before base64.
const planBundleMaxBytes = 64 << 20

// ExportPlan serves the plan as a .nib file.
func (h *DialogHandler) ExportPlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		bundle, err := h.chatSvc.ExportPlan(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		body, err := json.MarshalIndent(bundle, "", "  ")
		if err != nil {
			handleServiceError(w, fmt.Errorf("marshal plan bundle: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+planBundleFileName(bundle.Plan.Title)+`"`)
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(append(body, '\n')); err != nil {
			slog.Warn("write plan bundle", "dialog_id", id, "error", err)
		}
	}
}

// ImportPlan creates a new plan from a .nib file posted as the request body.
func (h *DialogHandler) ImportPlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requireJSONContentType(w, r) {
			return
		}
		var bundle service.PlanBundle
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, planBundleMaxBytes)).Decode(&bundle); err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("plan file exceeds %d MiB", planBundleMaxBytes>>20))
				return
			}
			writeError(w, http.StatusBadRequest, "plan file is not valid JSON")
			return
		}

		d, err := h.chatSvc.ImportPlan(r.Context(), bundle)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, d)
	}
}

// planBundleFileName is the download name for a plan: its title reduced to
// characters that are safe in a header and on every filesystem.
func planBundleFileName(title string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			sb.WriteRune(r)
			dash = false
			continue
		}
		if !dash && sb.Len() > 0 {
			sb.WriteByte('-')
			dash = true
		}
	}
	name := strings.TrimRight(sb.String(), "-")
	if len(name) > 80 {
		name = strings.TrimRight(name[:80], "-")
	}
	if name == "" {
		name = "plan"
	}
	return name + ".nib"
}

package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"strings"

	"zipserver/internal/comments"
	"zipserver/internal/notifications"
)

// getCreatorAndValidate checks session or returns mock creator if auth is disabled.
// Returns unauthorized status if user is not authenticated.
func (s *Server) getCreatorAndValidate(w http.ResponseWriter, r *http.Request) (comments.Creator, bool) {
	if s.auth == nil {
		// Return mock developer profile when auth is disabled
		return comments.Creator{
			ID:     "usr_dev_123",
			Type:   "Person",
			Name:   "Dev User",
			Avatar: "https://www.gravatar.com/avatar/00000000000000000000000000000000?d=mp&f=y",
			Email:  "dev@example.com",
		}, true
	}

	user, ok := s.auth.GetSessionUser(r)
	if !ok {
		http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
		return comments.Creator{}, false
	}

	return comments.Creator{
		ID:     user.ID,
		Type:   "Person",
		Name:   user.Name,
		Avatar: user.AvatarURL,
		Email:  user.Email,
	}, true
}

func (s *Server) HandleGetComments(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if s.auth != nil {
		_, ok := s.auth.GetSessionUser(r)
		if !ok {
			http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	pageParam := r.URL.Query().Get("page")
	if pageParam == "" {
		http.Error(w, `{"error":"Missing page query parameter"}`, http.StatusBadRequest)
		return
	}

	book, version, innerPath, err := comments.ParsePageParam(pageParam)
	if err != nil {
		slog.Warn("failed to parse page parameter", "page", pageParam, "error", err)
		http.Error(w, fmt.Sprintf(`{"error":"Invalid page parameter: %v"}`, err), http.StatusBadRequest)
		return
	}

	annos, err := s.comments.GetComments(r.Context(), book, version, innerPath)
	if err != nil {
		slog.Error("failed to retrieve comments", "book", book, "version", version, "path", innerPath, "error", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	// Always output valid json array
	if annos == nil {
		annos = []comments.Annotation{}
	}

	json.NewEncoder(w).Encode(annos)
}

func (s *Server) HandleCreateComment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	creator, ok := s.getCreatorAndValidate(w, r)
	if !ok {
		return
	}

	var input struct {
		Body struct {
			Value string `json:"value"`
		} `json:"body"`
		Target     comments.Target `json:"target"`
		Motivation string          `json:"motivation,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, `{"error":"Invalid request JSON"}`, http.StatusBadRequest)
		return
	}

	if input.Body.Value == "" {
		http.Error(w, `{"error":"Comment body value cannot be empty"}`, http.StatusBadRequest)
		return
	}

	// Figure out target source page URL/path
	var pageSource string
	pageParam := r.URL.Query().Get("page")

	if pageParam != "" {
		pageSource = pageParam
	} else if input.Target.Object != nil && input.Target.Object.Source != "" {
		pageSource = input.Target.Object.Source
	}

	var book, version, innerPath string
	var err error

	if pageSource == "" {
		// If no page source or page param was specified, it MUST be a reply.
		// We look up the parent annotation in the store to find the page context.
		if input.Motivation == "replying" && input.Target.String != "" {
			var found bool
			book, version, innerPath, found = s.comments.GetPageForComment(input.Target.String)
			if !found {
				http.Error(w, `{"error":"Parent annotation not found"}`, http.StatusNotFound)
				return
			}
		} else {
			http.Error(w, `{"error":"Missing target source or page parameter"}`, http.StatusBadRequest)
			return
		}
	} else {
		book, version, innerPath, err = comments.ParsePageParam(pageSource)
		if err != nil {
			slog.Warn("failed to parse page source", "source", pageSource, "error", err)
			http.Error(w, fmt.Sprintf(`{"error":"Invalid target source/page: %v"}`, err), http.StatusBadRequest)
			return
		}
	}

	anno := comments.Annotation{
		Creator:    creator,
		Body:       comments.TextualBody{Type: "TextualBody", Value: input.Body.Value, Format: "text/markdown"},
		Target:     input.Target,
		Motivation: input.Motivation,
	}

	// Non-reply annotations start as unresolved
	if input.Motivation != "replying" {
		isResolved := false
		anno.Resolved = &isResolved
	}

	created, err := s.comments.CreateComment(r.Context(), book, version, innerPath, &anno)
	if err != nil {
		slog.Error("failed to create comment", "book", book, "path", innerPath, "error", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	if s.mailer != nil {
		go s.sendCommentNotification(*created)
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(created)
}

func (s *Server) HandleUpdateComment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	creator, ok := s.getCreatorAndValidate(w, r)
	if !ok {
		return
	}

	id := getCommentID(r)
	if id == "" {
		http.Error(w, `{"error":"Missing comment ID"}`, http.StatusBadRequest)
		return
	}

	var payload struct {
		Body *struct {
			Value string `json:"value"`
		} `json:"body"`
		Resolved *bool `json:"resolved"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, `{"error":"Invalid request JSON"}`, http.StatusBadRequest)
		return
	}

	book, _, _, found := s.comments.GetPageForComment(id)
	if !found {
		http.Error(w, `{"error":"Comment not found"}`, http.StatusNotFound)
		return
	}

	updateFn := func(anno *comments.Annotation) error {
		if payload.Body != nil {
			if creator.ID != anno.Creator.ID {
				return fmt.Errorf("forbidden: creator mismatch")
			}
			anno.Body.Value = payload.Body.Value
		}
		if payload.Resolved != nil {
			anno.Resolved = payload.Resolved
		}
		return nil
	}

	updated, err := s.comments.UpdateComment(r.Context(), book, id, updateFn)
	if err != nil {
		if err.Error() == "forbidden: creator mismatch" {
			http.Error(w, `{"error":"Forbidden: only the creator can edit this comment"}`, http.StatusForbidden)
			return
		}
		slog.Error("failed to update comment", "id", id, "error", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(updated)
}

func (s *Server) HandleDeleteComment(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	creator, ok := s.getCreatorAndValidate(w, r)
	if !ok {
		return
	}

	id := getCommentID(r)
	if id == "" {
		http.Error(w, `{"error":"Missing comment ID"}`, http.StatusBadRequest)
		return
	}

	book, _, _, found := s.comments.GetPageForComment(id)
	if !found {
		http.Error(w, `{"error":"Comment not found"}`, http.StatusNotFound)
		return
	}

	err := s.comments.DeleteComment(r.Context(), book, id, creator.ID)
	if err != nil {
		if strings.Contains(err.Error(), "unauthorized") || strings.Contains(err.Error(), "mismatch") {
			http.Error(w, `{"error":"Forbidden: only the creator can delete this comment"}`, http.StatusForbidden)
			return
		}
		slog.Error("failed to delete comment", "id", id, "error", err)
		http.Error(w, `{"error":"Internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func getCommentID(r *http.Request) string {
	id := r.PathValue("id")
	if id == "" {
		id = strings.TrimPrefix(r.URL.Path, "/_/api/v1/comments/")
	}
	return id
}

func (s *Server) sendCommentNotification(anno comments.Annotation) {
	slog.Info("processing comment notification",
		"comment_id", anno.ID,
		"book", anno.Book,
		"version", anno.Version,
		"page", anno.Page,
		"creator", anno.Creator.Email,
		"motivation", anno.Motivation,
	)

	pageComments, err := s.comments.GetComments(context.Background(), anno.Book, anno.Version, anno.Page)
	if err != nil {
		slog.Error("failed to get page comments for notification", "comment_id", anno.ID, "error", err)
		return
	}

	recipients := notifications.GetNotificationRecipients(anno, pageComments, s.notifications.Watchers)
	slog.Info("notification recipients calculated",
		"comment_id", anno.ID,
		"recipient_count", len(recipients),
		"recipients", recipients,
	)
	if len(recipients) == 0 {
		return
	}

	subject := fmt.Sprintf("[Zipserver] New comment on %s (%s)", anno.Book, anno.Version)
	if anno.Motivation == "replying" {
		subject = fmt.Sprintf("[Zipserver] New reply on %s (%s)", anno.Book, anno.Version)
	}

	baseURL := s.notifications.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	pageURL := fmt.Sprintf("%s/%s/%s/%s", strings.TrimSuffix(baseURL, "/"), anno.Book, anno.Version, strings.TrimPrefix(anno.Page, "/"))

	var body strings.Builder
	body.WriteString("<html><body>")
	body.WriteString(fmt.Sprintf("<p><strong>%s</strong> added a comment to <strong>%s/%s/%s</strong>:</p>", html.EscapeString(anno.Creator.Name), html.EscapeString(anno.Book), html.EscapeString(anno.Version), html.EscapeString(anno.Page)))
	body.WriteString("<blockquote style=\"border-left: 4px solid #ccc; padding-left: 10px; margin: 10px 0; color: #555;\">")
	body.WriteString(strings.ReplaceAll(html.EscapeString(anno.Body.Value), "\n", "<br>"))
	body.WriteString("</blockquote>")
	body.WriteString(fmt.Sprintf("<p><a href=\"%s\">View Comment Thread</a></p>", pageURL))
	body.WriteString("</body></html>")

	slog.Info("sending notification email",
		"comment_id", anno.ID,
		"subject", subject,
		"recipients", recipients,
	)
	err = s.mailer.SendEmail(recipients, subject, body.String())
	if err != nil {
		slog.Error("failed to send comment email notification", "comment_id", anno.ID, "error", err)
	} else {
		slog.Info("comment email notification sent successfully", "comment_id", anno.ID)
	}
}

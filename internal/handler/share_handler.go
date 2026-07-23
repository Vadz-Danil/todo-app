package handler

import (
	"net/http"
	"strings"

	"todo-app/internal/models"
	"todo-app/internal/service"

	"github.com/gin-gonic/gin"
)

type ShareHandler struct {
	share     service.Share
	analytics service.Analytics
}

func NewShareHandler(share service.Share, analytics service.Analytics) *ShareHandler {
	return &ShareHandler{share: share, analytics: analytics}
}

type ShareLinkCreateRequest struct {
	Kind    string `json:"kind"`
	Label   string `json:"label"`
	TTLDays *int   `json:"ttl_days"`
}

func (h *ShareHandler) CreateLink(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	var req ShareLinkCreateRequest
	// An absent body is fine: it means "a board link that never expires".
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
	}

	link, err := h.share.CreateLink(c.Request.Context(), userID, service.ShareLinkInput{
		Kind:    models.ShareKind(strings.ToUpper(strings.TrimSpace(req.Kind))),
		Label:   req.Label,
		TTLDays: req.TTLDays,
	})
	if respondServiceError(c, err, "Failed to create share link") {
		return
	}

	c.JSON(http.StatusCreated, link)
}

func (h *ShareHandler) ListLinks(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	links, err := h.share.ListLinks(c.Request.Context(), userID)
	if respondServiceError(c, err, "Failed to fetch share links") {
		return
	}
	if links == nil {
		links = []models.ShareLink{}
	}

	c.JSON(http.StatusOK, gin.H{"links": links})
}

// RevokeLink kills a link but keeps the row, so the owner can still see that
// it existed and how often it was opened.
func (h *ShareHandler) RevokeLink(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	linkID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	if respondServiceError(c, h.share.RevokeLink(c.Request.Context(), userID, linkID), "Failed to revoke share link") {
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *ShareHandler) DeleteLink(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		return
	}

	linkID, ok := parsePathUUID(c, "id")
	if !ok {
		return
	}

	if respondServiceError(c, h.share.DeleteLink(c.Request.Context(), userID, linkID), "Failed to delete share link") {
		return
	}

	c.Status(http.StatusNoContent)
}

// Resolve serves a public link. It is the one route with no authentication:
// the token in the path is the credential.
func (h *ShareHandler) Resolve(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))

	q, ok := parseAnalyticsQuery(c, h.analytics)
	if !ok {
		return
	}

	view, err := h.share.Resolve(c.Request.Context(), token, q)
	if respondServiceError(c, err, "Failed to open share link") {
		return
	}

	// A shared board changes as its owner works, and the token must not linger
	// in a shared cache, so this response is never stored.
	c.Header("Cache-Control", "no-store, private")
	c.Header("Referrer-Policy", "no-referrer")
	// The token sits in the URL, so keep the page out of search results.
	c.Header("X-Robots-Tag", "noindex, nofollow")

	c.JSON(http.StatusOK, view)
}

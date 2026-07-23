package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"todo-app/internal/apperrors"
	"todo-app/internal/config"
	"todo-app/internal/models"
	"todo-app/internal/repository"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

const (
	exportUserAgent      = "TaskFlow-Exporter/1"
	exportNonceBytes     = 16
	maxTargetNameRunes   = 120
	maxDeliveryErrRunes  = 500
	exportInitialBackoff = 500 * time.Millisecond
	maxDrainBytes        = 64 << 10

	statusSuccess = "SUCCESS"
	statusFailed  = "FAILED"
)

type ExportService struct {
	repo      repository.ExportRepository
	analytics Analytics
	tasks     Task
	sprints   Sprint
	cfg       config.ExportConfig
	client    *http.Client
	logger    *zap.Logger
}

func NewExportService(
	repo repository.ExportRepository,
	analytics Analytics,
	tasks Task,
	sprints Sprint,
	cfg config.ExportConfig,
	logger *zap.Logger,
) *ExportService {
	return &ExportService{
		repo:      repo,
		analytics: analytics,
		tasks:     tasks,
		sprints:   sprints,
		cfg:       cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			// A webhook receiver must accept the POST itself. Following a
			// redirect would turn it into a bodyless GET and, worse, would
			// re-enter a host that never passed validateURL — so surface the
			// 3xx as the final response and let it be recorded as a failure.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				DialContext:           safeDialContext(cfg.AllowPrivate),
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: cfg.Timeout,
				MaxIdleConnsPerHost:   2,
			},
		},
		logger: logger,
	}
}

// safeDialContext enforces the SSRF policy at connect time rather than at
// validation time. Resolving here and dialling the vetted IP literal closes the
// gap validateURL alone cannot: a DNS record that changes between validation and
// the request (rebinding), and any host reached through a redirect.
func safeDialContext(allowPrivate bool) func(ctx context.Context, network, addr string) (net.Conn, error) {
	base := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}

	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}

		var ips []net.IP
		if literal := net.ParseIP(host); literal != nil {
			ips = []net.IP{literal}
		} else if ips, err = net.DefaultResolver.LookupIP(ctx, "ip", host); err != nil {
			return nil, err
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("%w: %s does not resolve", apperrors.ErrInvalidExportURL, host)
		}

		if !allowPrivate {
			for _, ip := range ips {
				if isBlockedIP(ip) {
					return nil, fmt.Errorf("%w: %s resolves to %s", apperrors.ErrBlockedExportURL, host, ip)
				}
			}
		}

		var lastErr error
		for _, ip := range ips {
			conn, err := base.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, lastErr
	}
}

// BuildEnvelope assembles the full snapshot and then keeps only the sections the
// kind asks for. Counts and Range always describe the whole window so a receiver
// can sanity-check a partial envelope without parsing it.
func (s *ExportService) BuildEnvelope(ctx context.Context, user *models.User, kind models.ExportKind, q models.AnalyticsQuery, summary *models.AISummary) (*models.ExportEnvelope, error) {
	if user == nil {
		return nil, apperrors.ErrUnauthorized
	}
	if !kind.IsValid() {
		return nil, apperrors.ErrInvalidExportKind
	}

	dashboard, err := s.analytics.Dashboard(ctx, user.ID, q)
	if err != nil {
		return nil, err
	}

	tasks, err := s.tasks.GetTasks(ctx, user.ID, models.TaskFilter{From: &q.From, To: &q.To})
	if err != nil {
		return nil, err
	}

	sprints, err := s.sprints.List(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	nonce, err := exportNonce()
	if err != nil {
		s.logger.Error("failed to generate export nonce", zap.Error(err))
		return nil, fmt.Errorf("generate export nonce: %w", err)
	}

	done := 0
	for i := range tasks {
		if tasks[i].Status == models.StatusDone {
			done++
		}
	}

	rangeInfo := dashboard.Range
	envelope := &models.ExportEnvelope{
		Schema:      models.ExportSchema,
		Kind:        kind,
		GeneratedAt: time.Now().UTC(),
		Nonce:       nonce,
		User:        models.ExportUser{ID: user.ID, Email: user.Email},
		Range:       &rangeInfo,
		Counts: map[string]int{
			"tasks":   len(tasks),
			"sprints": len(sprints),
			"open":    len(tasks) - done,
			"done":    done,
		},
	}

	switch kind {
	case models.ExportAnalytics:
		envelope.Analytics = dashboard
	case models.ExportTasks:
		envelope.Tasks = tasks
	case models.ExportSprints:
		envelope.Sprints = sprints
	case models.ExportAISummary:
		envelope.Summary = summary
	case models.ExportFull:
		envelope.Analytics = dashboard
		envelope.Tasks = tasks
		envelope.Sprints = sprints
		envelope.Summary = summary
	}

	return envelope, nil
}

func (s *ExportService) Push(ctx context.Context, user *models.User, req PushRequest) (*models.ExportDelivery, error) {
	if user == nil {
		return nil, apperrors.ErrUnauthorized
	}

	adHocURL := strings.TrimSpace(req.URL)
	if (req.TargetID != nil) == (adHocURL != "") {
		return nil, fmt.Errorf("%w: provide exactly one of target_id or url", apperrors.ErrInvalidExportURL)
	}

	var (
		targetID *uuid.UUID
		endpoint string
		secret   string
		headers  map[string]string
	)

	if req.TargetID != nil {
		target, err := s.repo.GetTarget(ctx, user.ID, *req.TargetID)
		if err != nil {
			if errors.Is(err, apperrors.ErrExportTargetNotFound) {
				return nil, err
			}
			s.logger.Error("failed to load export target",
				zap.Error(err), zap.String("user_id", user.ID.String()), zap.String("target_id", req.TargetID.String()))
			return nil, fmt.Errorf("get export target: %w", err)
		}
		if !target.Enabled {
			return nil, fmt.Errorf("%w: target %q is disabled", apperrors.ErrExportFailed, target.Name)
		}
		// Re-validate on every push: the row was checked when it was saved, but
		// the policy or the DNS record behind it may have changed since.
		validated, err := s.validateURL(target.URL)
		if err != nil {
			return nil, err
		}
		targetID = &target.ID
		endpoint, secret, headers = validated, target.Secret, target.Headers
	} else {
		validated, err := s.validateURL(adHocURL)
		if err != nil {
			return nil, err
		}
		endpoint, secret, headers = validated, strings.TrimSpace(req.Secret), req.Headers
	}

	envelope, err := s.BuildEnvelope(ctx, user, req.Kind, req.Query, req.Summary)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(envelope)
	if err != nil {
		s.logger.Error("failed to marshal export envelope", zap.Error(err), zap.String("user_id", user.ID.String()))
		return nil, fmt.Errorf("marshal export envelope: %w", err)
	}
	if s.cfg.MaxPayloadSize > 0 && len(body) > s.cfg.MaxPayloadSize {
		return nil, fmt.Errorf("%w: payload of %d bytes exceeds the %d byte limit",
			apperrors.ErrExportFailed, len(body), s.cfg.MaxPayloadSize)
	}

	startedAt := time.Now()
	statusCode, attempts, sendErr := s.deliver(ctx, endpoint, secret, headers, req.Kind, body)
	finishedAt := time.Now()

	delivery := &models.ExportDelivery{
		ID:          uuid.New(),
		UserID:      user.ID,
		TargetID:    targetID,
		URL:         endpoint,
		Kind:        req.Kind,
		Status:      statusSuccess,
		Attempts:    attempts,
		DurationMS:  int(finishedAt.Sub(startedAt).Milliseconds()),
		PayloadSize: len(body),
		CreatedAt:   finishedAt.UTC(),
	}
	if statusCode > 0 {
		delivery.StatusCode = &statusCode
	}

	var pushErr error
	if sendErr != nil {
		message := truncateRunes(sendErr.Error(), maxDeliveryErrRunes)
		delivery.Status = statusFailed
		delivery.Error = &message
		pushErr = fmt.Errorf("%w: %s", apperrors.ErrExportFailed, message)
		s.logger.Warn("export delivery failed",
			zap.String("user_id", user.ID.String()), zap.String("endpoint", redactEndpoint(endpoint)),
			zap.Int("attempts", attempts), zap.Int("status_code", statusCode), zap.Error(sendErr))
	}

	// The request may already be gone (client disconnect, handler timeout) but the
	// POST has happened, so the audit trail is written on a detached context.
	auditCtx := context.WithoutCancel(ctx)

	if err := s.repo.RecordDelivery(auditCtx, delivery); err != nil {
		s.logger.Error("failed to record export delivery",
			zap.Error(err), zap.String("user_id", user.ID.String()), zap.String("endpoint", redactEndpoint(endpoint)))
	}
	if targetID != nil {
		if err := s.repo.TouchTarget(auditCtx, user.ID, *targetID, delivery.StatusCode, delivery.Error, delivery.CreatedAt); err != nil {
			s.logger.Error("failed to touch export target",
				zap.Error(err), zap.String("user_id", user.ID.String()), zap.String("target_id", targetID.String()))
		}
	}

	return delivery, pushErr
}

func (s *ExportService) CreateTarget(ctx context.Context, userID uuid.UUID, in TargetInput) (*models.ExportTarget, error) {
	if in.Name == nil || in.URL == nil {
		return nil, fmt.Errorf("%w: name and url are required", apperrors.ErrInvalidExportURL)
	}

	name, err := validateTargetName(*in.Name)
	if err != nil {
		return nil, err
	}

	endpoint, err := s.validateURL(*in.URL)
	if err != nil {
		return nil, err
	}

	secret := ""
	if in.Secret != nil {
		secret = strings.TrimSpace(*in.Secret)
	}

	target := &models.ExportTarget{
		ID:      uuid.New(),
		UserID:  userID,
		Name:    name,
		URL:     endpoint,
		Secret:  secret,
		Headers: normalizeExportHeaders(in.Headers),
		Enabled: in.Enabled == nil || *in.Enabled,
	}

	if err := s.repo.CreateTarget(ctx, target); err != nil {
		s.logger.Error("failed to create export target", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("create export target: %w", err)
	}

	return hideSecret(target), nil
}

func (s *ExportService) ListTargets(ctx context.Context, userID uuid.UUID) ([]models.ExportTarget, error) {
	targets, err := s.repo.ListTargets(ctx, userID)
	if err != nil {
		s.logger.Error("failed to list export targets", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list export targets: %w", err)
	}
	for i := range targets {
		hideSecret(&targets[i])
	}
	return targets, nil
}

func (s *ExportService) UpdateTarget(ctx context.Context, userID, targetID uuid.UUID, in TargetInput) (*models.ExportTarget, error) {
	target, err := s.repo.GetTarget(ctx, userID, targetID)
	if err != nil {
		if errors.Is(err, apperrors.ErrExportTargetNotFound) {
			return nil, err
		}
		s.logger.Error("failed to load export target",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("target_id", targetID.String()))
		return nil, fmt.Errorf("get export target: %w", err)
	}

	if in.Name != nil {
		name, err := validateTargetName(*in.Name)
		if err != nil {
			return nil, err
		}
		target.Name = name
	}

	if in.URL != nil {
		endpoint, err := s.validateURL(*in.URL)
		if err != nil {
			return nil, err
		}
		target.URL = endpoint
	}
	// A nil map means the field was absent from the patch; only an explicitly
	// supplied (possibly empty) map replaces the stored headers.
	if in.Headers != nil {
		target.Headers = normalizeExportHeaders(in.Headers)
	}
	if in.Secret != nil {
		target.Secret = strings.TrimSpace(*in.Secret)
	}
	if in.Enabled != nil {
		target.Enabled = *in.Enabled
	}

	if err := s.repo.UpdateTarget(ctx, target); err != nil {
		if errors.Is(err, apperrors.ErrExportTargetNotFound) {
			return nil, err
		}
		s.logger.Error("failed to update export target",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("target_id", targetID.String()))
		return nil, fmt.Errorf("update export target: %w", err)
	}

	return hideSecret(target), nil
}

func (s *ExportService) DeleteTarget(ctx context.Context, userID, targetID uuid.UUID) error {
	if err := s.repo.DeleteTarget(ctx, userID, targetID); err != nil {
		if errors.Is(err, apperrors.ErrExportTargetNotFound) {
			return err
		}
		s.logger.Error("failed to delete export target",
			zap.Error(err), zap.String("user_id", userID.String()), zap.String("target_id", targetID.String()))
		return fmt.Errorf("delete export target: %w", err)
	}
	return nil
}

func (s *ExportService) ListDeliveries(ctx context.Context, userID uuid.UUID, limit int) ([]models.ExportDelivery, error) {
	deliveries, err := s.repo.ListDeliveries(ctx, userID, limit)
	if err != nil {
		s.logger.Error("failed to list export deliveries", zap.Error(err), zap.String("user_id", userID.String()))
		return nil, fmt.Errorf("list export deliveries: %w", err)
	}
	return deliveries, nil
}

// deliver POSTs body, retrying network errors, 429 and 5xx. It returns the last
// observed status code (0 when no response was received) and how many attempts
// were made.
func (s *ExportService) deliver(ctx context.Context, endpoint, secret string, headers map[string]string, kind models.ExportKind, body []byte) (int, int, error) {
	maxAttempts := s.cfg.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	backoff := exportInitialBackoff

	for attempt := 1; ; attempt++ {
		// status is 0 when the attempt never got a response, and the audit trail
		// must report THIS attempt rather than carry a code over from an earlier
		// one that did reach the target.
		status, err := s.attempt(ctx, endpoint, secret, headers, kind, body)
		if err == nil {
			if status >= http.StatusOK && status < http.StatusMultipleChoices {
				return status, attempt, nil
			}
			err = fmt.Errorf("target responded with status %d", status)
		}

		retryable := status == 0 || status == http.StatusTooManyRequests || status >= http.StatusInternalServerError
		if !retryable || attempt >= maxAttempts {
			return status, attempt, err
		}

		select {
		case <-ctx.Done():
			return status, attempt, errors.Join(err, ctx.Err())
		case <-time.After(backoff):
		}
		backoff *= 2
	}
}

func (s *ExportService) attempt(ctx context.Context, endpoint, secret string, headers map[string]string, kind models.ExportKind, body []byte) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	signRequest(request, secret, headers, kind, body)

	response, err := s.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxDrainBytes))
		_ = response.Body.Close()
	}()

	return response.StatusCode, nil
}

// signRequest sets the protocol headers first so a custom header can override
// things like User-Agent, but never Content-Type or the X-TaskFlow-* contract.
func signRequest(request *http.Request, secret string, headers map[string]string, kind models.ExportKind, body []byte) {
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", exportUserAgent)
	request.Header.Set("X-TaskFlow-Event", string(kind))
	request.Header.Set("X-TaskFlow-Timestamp", timestamp)

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(timestamp + "."))
		mac.Write(body)
		request.Header.Set("X-TaskFlow-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}

	for key, value := range headers {
		if key = strings.TrimSpace(key); key == "" || isReservedExportHeader(key) {
			continue
		}
		request.Header.Set(key, value)
	}
}

// validateURL enforces the outbound SSRF policy: absolute http(s) only, and
// unless private targets are allowed, nothing that resolves into a local or
// private network.
func (s *ExportService) validateURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", apperrors.ErrInvalidExportURL
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", apperrors.ErrInvalidExportURL
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", apperrors.ErrInvalidExportURL
	}

	host := parsed.Hostname()
	if host == "" {
		return "", apperrors.ErrInvalidExportURL
	}
	if s.cfg.AllowPrivate {
		return trimmed, nil
	}

	if literal := net.ParseIP(host); literal != nil {
		if isBlockedIP(literal) {
			return "", apperrors.ErrBlockedExportURL
		}
		return trimmed, nil
	}

	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return "", apperrors.ErrInvalidExportURL
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return "", apperrors.ErrBlockedExportURL
		}
	}

	return trimmed, nil
}

// nat64Prefix is the well-known 64:ff9b::/96 range, which embeds an IPv4
// address that would otherwise reach a private host through a translator.
var nat64Prefix = net.IPNet{
	IP:   net.IP{0x00, 0x64, 0xff, 0x9b, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	Mask: net.CIDRMask(96, 128),
}

// cgnatRange is RFC 6598 100.64.0.0/10, which net.IP.IsPrivate does not cover.
var cgnatRange = net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast() {
		return true
	}

	// IPv6 unique-local (fc00::/7) and the deprecated site-local (fec0::/10)
	// range, neither of which IsPrivate reports for a 16-byte address.
	if v6 := ip.To16(); v6 != nil && ip.To4() == nil {
		if v6[0]&0xfe == 0xfc || (v6[0] == 0xfe && v6[1]&0xc0 == 0xc0) {
			return true
		}
		if nat64Prefix.Contains(ip) {
			return isBlockedIP(net.IPv4(v6[12], v6[13], v6[14], v6[15]))
		}
		// IPv4-compatible IPv6 (::a.b.c.d) tunnels an IPv4 address that To4
		// refuses to unwrap, so check the embedded address directly.
		if isZeroPrefix(v6[:12]) && !isZeroPrefix(v6[12:]) {
			return isBlockedIP(net.IPv4(v6[12], v6[13], v6[14], v6[15]))
		}
	}

	if v4 := ip.To4(); v4 != nil && cgnatRange.Contains(v4) {
		return true
	}

	return false
}

// redactEndpoint keeps only scheme and host for logs. A webhook URL often
// carries its own token in the path or query, so it is a credential.
func redactEndpoint(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "invalid-url"
	}
	return parsed.Scheme + "://" + parsed.Host
}

func isZeroPrefix(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

func validateTargetName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	length := utf8.RuneCountInString(name)
	if length == 0 || length > maxTargetNameRunes {
		return "", fmt.Errorf("%w: target name must be 1 to %d characters", apperrors.ErrInvalidExportURL, maxTargetNameRunes)
	}
	return name, nil
}

func normalizeExportHeaders(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" || isReservedExportHeader(key) {
			continue
		}
		out[key] = value
	}
	return out
}

func isReservedExportHeader(key string) bool {
	lower := strings.ToLower(key)
	return lower == "content-type" || strings.HasPrefix(lower, "x-taskflow-")
}

func hideSecret(target *models.ExportTarget) *models.ExportTarget {
	target.HasSecret = strings.TrimSpace(target.Secret) != ""
	target.Secret = ""
	return target
}

func exportNonce() (string, error) {
	buf := make([]byte, exportNonceBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

# TaskFlow HTTP API

All `/api/*` routes require `Authorization: Bearer <access_token>`.
Errors are always `{"error": "human readable message"}` with a matching status code.
Timestamps are RFC3339. Money-free numerics (hours) are JSON numbers.

## Auth (unchanged)

| Method | Path | Body |
| --- | --- | --- |
| POST | `/auth/register` | `{email, password}` |
| POST | `/auth/login` | `{email, password}` → `{access_token, refresh_token}` |
| POST | `/auth/google` | `{code, redirect_uri?}` — `redirect_uri` is required by the redirect flow and must be on `GOOGLE_ALLOWED_REDIRECT_URLS`; omitting it falls back to the popup flow's `postmessage` |
| POST | `/auth/refresh` | `{refresh_token}` |
| GET | `/api/me` | → `{id, email, created_at}` |

## Tasks

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/api/tasks` | Query: `status` (CSV), `priority` (CSV), `sprint_id`, `q`, `from`, `to`. → `{tasks: Task[]}` sorted by status then `position` asc |
| POST | `/api/tasks` | → `201 Task` |
| GET | `/api/tasks/:id` | → `Task` |
| PATCH | `/api/tasks/:id` | Partial update. Present-and-`null` clears a nullable field; absent leaves it alone. → `Task` |
| PATCH | `/api/tasks/:id/status` | `{status, reviewer?}` → `{message, task}` |
| PATCH | `/api/tasks/:id/move` | Kanban drag. `{status, after_id?, before_id?}` → `Task` |
| DELETE | `/api/tasks/:id` | → `204` |
| POST | `/api/tasks/share` | `{recipient_email}` (unchanged) |

**Create/patch body**

```jsonc
{
  "title": "string (required on create)",
  "description": "string|null",
  "status": "TODO|IN_PROGRESS|IN_REVIEW|DONE",
  "priority": "LOW|MEDIUM|HIGH|URGENT",
  "reviewer": "string|null",       // REQUIRED and non-empty when status == IN_REVIEW
  "estimate_hours": 4.5,           // number|null, >= 0, <= 1000
  "buffer_hours": 1.5,             // number|null, >= 0, <= 1000
  "spent_hours": 2,                // number|null (patch only)
  "blockers": "string|null",
  "sprint_id": "uuid|null",
  "due_date": "2026-08-01T00:00:00Z" // string|null
}
```

**Validation rules**

- `400 invalid task status` / `invalid task priority` for unknown enum values.
- `400 reviewer is required when status is IN_REVIEW` when moving to `IN_REVIEW`
  without a reviewer already set and without one supplied.
- Moving out of `IN_REVIEW` keeps the reviewer (it is history, not a lock).
- Entering `IN_PROGRESS`/`IN_REVIEW`/`DONE` for the first time stamps `started_at`.
- Entering `DONE` stamps `completed_at`; leaving `DONE` clears it.
- Every status transition appends a `task_status_history` row.

**`/move` semantics** — `after_id` is the task that ends up directly ABOVE the moved
card, `before_id` the one directly BELOW. Both optional: neither = append to bottom
of `status`; only `before_id` = insert at top.

## Analytics

`GET /api/analytics/dashboard`

| Param | Default | Values |
| --- | --- | --- |
| `period` | `week` | `today`, `week`, `month`, `quarter`, `half_year`, `year`, `all_time`, `custom` |
| `from`, `to` | – | Required when `period=custom`. `YYYY-MM-DD` or RFC3339 |
| `granularity` | derived | `day` (≤ 45d), `week` (≤ 400d), `month` |
| `tz` | `UTC` | IANA name, e.g. `Europe/Kyiv` |

→ `200 Dashboard` (see `internal/models/analytics.go` / `frontend/src/types/index.ts`).

`GET /api/analytics/export?format=json|csv&…` — same params, returns the dashboard as
a downloadable file (`Content-Disposition: attachment`).

## AI (Gemini)

Returns `503 {"error": "AI features are disabled: GEMINI_API_KEY is not configured"}`
when unconfigured; `502` when the provider misbehaves.

| Method | Path | Body / Notes |
| --- | --- | --- |
| GET | `/api/ai/status` | → `{enabled: bool, model: string}` |
| POST | `/api/ai/summary` | `{period, from?, to?, granularity?, tz?, refresh?}` → `AISummary` (cached by content fingerprint unless `refresh`) |
| GET | `/api/ai/summaries?limit=20` | → `{summaries: AISummary[]}` |
| POST | `/api/ai/planning/sessions` | `{raw_tasks: string[], notes?, horizon_weeks=1, capacity_hours_per_week=40, starts_on?, include_backlog?}` → `PlanningSession` with `state=COLLECTING` and generated `payload.questions` |
| GET | `/api/ai/planning/sessions?limit=20` | → `{sessions: PlanningSession[]}` |
| GET | `/api/ai/planning/sessions/:id` | → `PlanningSession` |
| POST | `/api/ai/planning/sessions/:id/answers` | `{answers: [{question_id, answer}]}` → session; may append follow-up questions, flips to `READY` when every item is resolved |
| POST | `/api/ai/planning/sessions/:id/plan` | → session with `state=PLANNED` and `payload.plan` |
| POST | `/api/ai/planning/sessions/:id/commit` | Creates the sprint + tasks → `{sprint, tasks}`, session `state=COMMITTED` |
| DELETE | `/api/ai/planning/sessions/:id` | → `204` |

The planner always asks, per item: blockers, effort estimate, and the safety buffer
the user wants on top of it — those three topics are mandatory before `READY`.

## Sprints

| Method | Path |
| --- | --- |
| GET | `/api/sprints` → `{sprints: Sprint[]}` (with `stats`) |
| POST | `/api/sprints` `{name, goal?, starts_on, ends_on, capacity_hours?, status?}` |
| GET | `/api/sprints/:id` → `Sprint` with `tasks` + `stats` |
| PATCH | `/api/sprints/:id` |
| DELETE | `/api/sprints/:id` → `204` (tasks are detached, not deleted) |

## Export ("push my data to some server")

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/api/export/targets` | → `{targets: ExportTarget[]}` — secrets never returned, only `has_secret` |
| POST | `/api/export/targets` | `{name, url, secret?, headers?, enabled?}` |
| PATCH | `/api/export/targets/:id` | same fields, all optional |
| DELETE | `/api/export/targets/:id` | → `204` |
| GET | `/api/export/preview` | `kind` + analytics params → the exact `ExportEnvelope` that would be sent |
| POST | `/api/export/push` | `{target_id?}` **or** `{url, secret?, headers?}`, plus `kind` and analytics params → `ExportDelivery` |
| GET | `/api/export/deliveries?limit=50` | → `{deliveries: ExportDelivery[]}` |

`kind` ∈ `ANALYTICS_SNAPSHOT | TASKS | SPRINTS | AI_SUMMARY | FULL` (default `ANALYTICS_SNAPSHOT`).

**Outbound request** — `POST <url>`, `Content-Type: application/json`, body is
`ExportEnvelope`, plus headers:

```
X-TaskFlow-Event: <kind>
X-TaskFlow-Timestamp: <unix seconds>
X-TaskFlow-Signature: sha256=<hex HMAC of "<timestamp>.<body>" using the target secret>
User-Agent: TaskFlow-Exporter/1
```

Retried with exponential backoff on 5xx/429/network errors (`EXPORT_MAX_RETRIES`,
default 2). Every attempt set is logged to `export_deliveries`. Targets resolving to
private/loopback addresses are rejected with `400` unless
`EXPORT_ALLOW_PRIVATE_TARGETS=true`.

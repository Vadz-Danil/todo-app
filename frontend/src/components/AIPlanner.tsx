import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
    ArrowLeft,
    CalendarDays,
    Check,
    CheckCircle2,
    ChevronDown,
    ChevronRight,
    Clock,
    Flag,
    Gauge,
    History,
    Info,
    Layers,
    Lightbulb,
    Link2,
    ListChecks,
    Loader2,
    MessageSquare,
    Plus,
    RefreshCw,
    RotateCcw,
    Send,
    ShieldAlert,
    Sparkles,
    Target,
    Trash2,
    TriangleAlert,
    Users,
    Zap,
} from 'lucide-react';
import {
    answerPlanning,
    commitPlan,
    deletePlanningSession,
    generatePlan,
    getAIStatus,
    getPlanningSession,
    listPlanningSessions,
    startPlanning,
} from '../api/endpoints';
import type { AILang, AIStatus } from '../api/endpoints';
import type {
    PlannedTask,
    PlannedWeek,
    PlanningItem,
    PlanningQuestion,
    PlanningSession,
    PlanningState,
    Sprint,
    Task,
    TaskPriority,
} from '../types';
import { PRIORITY_LABELS } from '../types';
import { useToast } from '../context/ToastContext';

interface AIPlannerProps {
    onCommitted?: () => void;
}

/* --------------------------------- helpers ---------------------------------- */

interface HttpErrorLike {
    response?: { status?: number; data?: { error?: string; message?: string } };
    message?: string;
}

const asHttpError = (err: unknown): HttpErrorLike =>
    typeof err === 'object' && err !== null ? (err as HttpErrorLike) : {};

const serverMessage = (err: unknown): string => {
    const data = asHttpError(err).response?.data;
    return data?.error ?? data?.message ?? '';
};

const friendlyError = (err: unknown, fallback: string): string => {
    const status = asHttpError(err).response?.status;
    if (status === 502 || status === 503) {
        return 'The AI service is busy or unreachable. Give it a few seconds and try again.';
    }
    const raw = serverMessage(err);
    return raw || fallback;
};

const pad = (n: number): string => String(n).padStart(2, '0');

const toISODate = (d: Date): string => `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;

/** The Monday after today — Monday itself rolls forward a full week. */
const nextMonday = (): string => {
    const d = new Date();
    const delta = (8 - d.getDay()) % 7 || 7;
    d.setDate(d.getDate() + delta);
    return toISODate(d);
};

/** Parses `YYYY-MM-DD` in local time so the day never slips a timezone. */
const parseISODate = (iso?: string): Date | null => {
    if (!iso) return null;
    const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(iso);
    if (!m) {
        const loose = new Date(iso);
        return Number.isNaN(loose.getTime()) ? null : loose;
    }
    return new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
};

const DAY_FMT = new Intl.DateTimeFormat('en-GB', { weekday: 'short', day: 'numeric', month: 'short' });
const STAMP_FMT = new Intl.DateTimeFormat('en-GB', {
    day: 'numeric',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
});

const formatDay = (iso?: string): string => {
    const d = parseISODate(iso);
    return d ? DAY_FMT.format(d) : '—';
};

const formatStamp = (iso?: string): string => {
    if (!iso) return '—';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '—' : STAMP_FMT.format(d);
};

const formatRange = (from?: string, to?: string): string => `${formatDay(from)} – ${formatDay(to)}`;

const round1 = (n: number): number => Math.round(n * 10) / 10;

const formatHours = (n?: number): string => {
    if (typeof n !== 'number' || !Number.isFinite(n)) return '—';
    const r = round1(n);
    return Number.isInteger(r) ? `${r}h` : `${r.toFixed(1)}h`;
};

const isAnswered = (q: PlanningQuestion): boolean => Boolean(q.answer && q.answer.trim());

const PRIORITY_CHIP: Record<TaskPriority, string> = {
    URGENT: 'border-danger/30 bg-danger/10 text-danger',
    HIGH: 'border-warning/30 bg-warning/10 text-warning',
    MEDIUM: 'border-info/30 bg-info/10 text-info',
    LOW: 'border-border bg-hover text-text',
};

const STATE_CHIP: Record<PlanningState, string> = {
    COLLECTING: 'border-info/30 bg-info/10 text-info',
    READY: 'border-warning/30 bg-warning/10 text-warning',
    PLANNED: 'border-accent-border bg-accent-bg text-accent',
    COMMITTED: 'border-success/30 bg-success/10 text-success',
};

const STATE_LABEL: Record<PlanningState, string> = {
    COLLECTING: 'Questions',
    READY: 'Ready',
    PLANNED: 'Planned',
    COMMITTED: 'On the board',
};

const STEPS = ['Setup', 'Q & A', 'Review', 'Plan'];

const stepOf = (state?: PlanningState): number => {
    if (state === 'COLLECTING') return 1;
    if (state === 'READY') return 2;
    if (state === 'PLANNED' || state === 'COMMITTED') return 3;
    return 0;
};

/* ------------------------------ small building blocks ------------------------ */

const MicroLabel: React.FC<{ children: React.ReactNode }> = ({ children }) => (
    <span className="text-[10px] font-mono uppercase tracking-wider text-text/70">{children}</span>
);

const Chip: React.FC<{ className?: string; children: React.ReactNode }> = ({ className = '', children }) => (
    <span
        className={`inline-flex items-center gap-1 rounded-lg border px-2 py-0.5 text-[10px] font-mono font-medium uppercase tracking-wider ${className}`}
    >
        {children}
    </span>
);

const PriorityChip: React.FC<{ priority?: TaskPriority }> = ({ priority }) => {
    if (!priority) return <span className="text-xs text-text/60">—</span>;
    return (
        <Chip className={PRIORITY_CHIP[priority] ?? PRIORITY_CHIP.LOW}>
            <Flag className="h-3 w-3" />
            {PRIORITY_LABELS[priority] ?? priority}
        </Chip>
    );
};

interface LoadBarProps {
    value: number;
    max: number;
    label?: string;
}

const LoadBar: React.FC<LoadBarProps> = ({ value, max, label }) => {
    const pct = max > 0 ? (value / max) * 100 : 0;
    const over = pct > 100;
    const width = max > 0 ? Math.min(100, Math.max(pct, value > 0 ? 3 : 0)) : 0;
    return (
        <div
            className="h-2 w-full overflow-hidden rounded-full bg-hover"
            role="progressbar"
            aria-label={label ?? 'Load'}
            aria-valuenow={Math.round(pct)}
            aria-valuemin={0}
            aria-valuemax={100}
        >
            <div
                className="h-full rounded-full transition-all duration-500"
                style={{ width: `${width}%`, background: over ? 'var(--viz-warning)' : 'var(--viz-1)' }}
            />
        </div>
    );
};

interface BusyPanelProps {
    label: string;
    hint: string;
}

/** Owns its own clock so a 10-30s AI call never looks frozen. Give it a `key`
 *  per phase so the counter restarts when the work changes. */
const BusyPanel: React.FC<BusyPanelProps> = ({ label, hint }) => {
    const [elapsed, setElapsed] = useState(0);

    useEffect(() => {
        const timer = window.setInterval(() => setElapsed((s) => s + 1), 1000);
        return () => window.clearInterval(timer);
    }, []);

    return (
        <div
            className="flex flex-col gap-2 rounded-2xl border border-accent-border bg-accent-bg p-4 sm:flex-row sm:items-center sm:justify-between"
            aria-live="polite"
        >
            <div className="flex min-w-0 items-center gap-3">
                <Loader2 className="h-4 w-4 shrink-0 animate-spin text-accent" />
                <div className="min-w-0">
                    <p className="text-sm font-semibold leading-snug text-text-h">{label}</p>
                    <p className="mt-0.5 text-xs text-text">{hint}</p>
                </div>
            </div>
            <span className="shrink-0 font-mono text-[10px] uppercase tracking-wider text-text/70">
                {elapsed}s · usually 10-30s
            </span>
        </div>
    );
};

const SkeletonRows: React.FC<{ rows?: number }> = ({ rows = 3 }) => (
    <div className="space-y-2">
        {Array.from({ length: rows }, (_, i) => (
            <div key={i} className="h-10 animate-pulse rounded-xl bg-hover" />
        ))}
    </div>
);

interface StatTileProps {
    label: string;
    value: string;
    icon: React.ComponentType<{ className?: string }>;
    tone?: 'default' | 'warn';
}

const StatTile: React.FC<StatTileProps> = ({ label, value, icon: Icon, tone = 'default' }) => (
    <div className="rounded-xl border border-border bg-bg p-3">
        <div className="flex items-center gap-1.5 text-text/70">
            <Icon className="h-3.5 w-3.5" />
            <MicroLabel>{label}</MicroLabel>
        </div>
        <p
            className="mt-1 text-lg font-semibold text-text-h"
            style={tone === 'warn' ? { color: 'var(--viz-warning)' } : undefined}
        >
            {value}
        </p>
    </div>
);

/* ------------------------------------ Q & A --------------------------------- */

interface QuestionBlockProps {
    question: PlanningQuestion;
    draft: string;
    disabled: boolean;
    onDraft: (id: string, value: string) => void;
}

const QuestionBlock: React.FC<QuestionBlockProps> = ({ question, draft, disabled, onDraft }) => {
    const [open, setOpen] = useState(false);
    const answered = isAnswered(question);
    const suggestions = question.suggestions ?? [];

    if (answered) {
        return (
            <div className="rounded-xl border border-border bg-bg">
                <button
                    type="button"
                    onClick={() => setOpen((v) => !v)}
                    aria-expanded={open}
                    className="flex w-full items-start gap-2 p-3 text-left transition hover:bg-hover active:scale-[0.995]"
                >
                    <CheckCircle2 className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" />
                    <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-1.5">
                            <Chip className="border-border bg-hover text-text">{question.topic}</Chip>
                            <span className="line-clamp-1 text-xs text-text">{question.question}</span>
                        </span>
                        <span className="mt-1 block truncate text-xs font-medium text-text-h">
                            {question.answer}
                        </span>
                    </span>
                    <ChevronDown
                        className={`mt-0.5 h-3.5 w-3.5 shrink-0 text-text/60 transition-transform ${open ? 'rotate-180' : ''}`}
                    />
                </button>

                {open && (
                    <div className="border-t border-border/60 px-3 pb-3 pt-2">
                        <p className="text-xs text-text-h">{question.question}</p>
                        {question.why && <p className="mt-1 text-xs text-text/70">{question.why}</p>}
                        <p className="mt-2 rounded-lg bg-code-bg px-2.5 py-2 text-xs text-text-h">
                            {question.answer}
                        </p>
                        {question.answered_at && (
                            <p className="mt-1.5 font-mono text-[10px] uppercase tracking-wider text-text/60">
                                Answered {formatStamp(question.answered_at)}
                            </p>
                        )}
                    </div>
                )}
            </div>
        );
    }

    return (
        <div className="rounded-xl border border-border bg-bg p-3">
            <div className="flex flex-wrap items-center gap-1.5">
                <Chip className="border-accent-border bg-accent-bg text-accent">{question.topic}</Chip>
                {draft.trim() && (
                    <Chip className="border-success/30 bg-success/10 text-success">Ready to send</Chip>
                )}
            </div>

            <p className="mt-2 text-sm font-medium leading-snug text-text-h">{question.question}</p>
            {question.why && <p className="mt-1 text-xs leading-relaxed text-text/70">{question.why}</p>}

            {suggestions.length > 0 && (
                <div className="mt-2 flex flex-wrap gap-1.5">
                    {suggestions.map((s, i) => (
                        <button
                            key={`${question.id}-s-${i}`}
                            type="button"
                            disabled={disabled}
                            onClick={() => onDraft(question.id, s)}
                            className="rounded-lg border border-border bg-surface px-2 py-1 text-[11px] text-text transition hover:border-accent-border hover:text-text-h active:scale-95 disabled:opacity-50"
                        >
                            {s}
                        </button>
                    ))}
                </div>
            )}

            <input
                type="text"
                value={draft}
                disabled={disabled}
                onChange={(e) => onDraft(question.id, e.target.value)}
                placeholder="Your answer..."
                aria-label={question.question}
                className="mt-2.5 w-full rounded-xl border border-border bg-surface px-3 py-2 text-xs text-text-h outline-none transition placeholder:text-text/50 focus:border-accent disabled:opacity-50"
            />
        </div>
    );
};

/* ---------------------------------- plan cards ------------------------------- */

interface PlannedTaskCardProps {
    task: PlannedTask;
    titleByRef: Record<string, string>;
}

const PlannedTaskCard: React.FC<PlannedTaskCardProps> = ({ task, titleByRef }) => {
    const subtasks = task.subtasks ?? [];
    const deps = task.depends_on ?? [];
    const total = (task.estimate_hours ?? 0) + (task.buffer_hours ?? 0);

    return (
        <div className="rounded-xl border border-border bg-bg p-3 transition hover:bg-hover">
            <div className="flex items-start justify-between gap-2">
                <h5 className="min-w-0 text-sm font-semibold leading-snug text-text-h">{task.title}</h5>
                <PriorityChip priority={task.priority} />
            </div>

            {task.description && (
                <p className="mt-1.5 text-xs leading-relaxed text-text">{task.description}</p>
            )}

            <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
                <Chip className="border-border bg-hover text-text">
                    <Clock className="h-3 w-3" />
                    {formatHours(task.estimate_hours)}
                    {task.buffer_hours ? ` + ${formatHours(task.buffer_hours)}` : ''}
                </Chip>
                {total > 0 && (
                    <Chip className="border-border bg-hover text-text/80">Total {formatHours(total)}</Chip>
                )}
                {task.needs_review && (
                    <Chip className="border-info/30 bg-info/10 text-info">
                        <Users className="h-3 w-3" />
                        {task.reviewer || 'Review'}
                    </Chip>
                )}
            </div>

            {deps.length > 0 && (
                <div className="mt-2 flex flex-wrap items-center gap-1.5">
                    <Link2 className="h-3 w-3 text-text/60" />
                    {deps.map((ref) => (
                        <span
                            key={`${task.ref}-dep-${ref}`}
                            className="rounded-lg border border-border bg-surface px-2 py-0.5 text-[11px] text-text"
                        >
                            {titleByRef[ref] ?? ref}
                        </span>
                    ))}
                </div>
            )}

            {task.blockers && (
                <p
                    className="mt-2 flex items-start gap-1.5 text-xs"
                    style={{ color: 'var(--viz-warning)' }}
                >
                    <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>{task.blockers}</span>
                </p>
            )}

            {subtasks.length > 0 && (
                <ul className="mt-2.5 space-y-1 border-t border-border/60 pt-2">
                    {subtasks.map((s, i) => (
                        <li key={`${task.ref}-st-${i}`} className="flex items-center gap-2 text-xs text-text">
                            <span className="h-3 w-3 shrink-0 rounded-[4px] border border-border bg-surface" />
                            <span className="min-w-0 flex-1 truncate text-text-h">{s.title}</span>
                            <span className="shrink-0 font-mono text-[10px] text-text/70">
                                {formatHours(s.estimate_hours)}
                            </span>
                        </li>
                    ))}
                </ul>
            )}
        </div>
    );
};

interface WeekCardProps {
    week: PlannedWeek;
    titleByRef: Record<string, string>;
}

const WeekCard: React.FC<WeekCardProps> = ({ week, titleByRef }) => {
    const tasks = week.tasks ?? [];
    const over = week.capacity_hours > 0 && week.load_hours > week.capacity_hours;

    return (
        <section className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
            <div className="flex flex-wrap items-start justify-between gap-2">
                <div className="min-w-0">
                    <div className="flex items-center gap-2">
                        <Chip className="border-border bg-hover text-text">Week {week.index}</Chip>
                        <span className="text-xs text-text">{formatRange(week.starts_on, week.ends_on)}</span>
                    </div>
                    {week.focus && (
                        <p className="mt-1.5 text-sm font-medium leading-snug text-text-h">{week.focus}</p>
                    )}
                </div>
                <div className="w-full sm:w-44">
                    <div className="flex items-center justify-between gap-2">
                        <MicroLabel>Load</MicroLabel>
                        <span
                            className="font-mono text-[11px] text-text-h"
                            style={over ? { color: 'var(--viz-warning)' } : undefined}
                        >
                            {formatHours(week.load_hours)} / {formatHours(week.capacity_hours)}
                        </span>
                    </div>
                    <div className="mt-1.5">
                        <LoadBar
                            value={week.load_hours}
                            max={week.capacity_hours}
                            label={`Week ${week.index} load`}
                        />
                    </div>
                </div>
            </div>

            {tasks.length === 0 ? (
                <p className="mt-3 rounded-xl border border-dashed border-border p-4 text-center text-xs text-text/70">
                    No tasks scheduled in this week.
                </p>
            ) : (
                <div className="mt-3 grid gap-2.5 lg:grid-cols-2">
                    {tasks.map((t) => (
                        <PlannedTaskCard key={`${week.index}-${t.ref}`} task={t} titleByRef={titleByRef} />
                    ))}
                </div>
            )}
        </section>
    );
};

interface BulletListProps {
    title: string;
    icon: React.ComponentType<{ className?: string; style?: React.CSSProperties }>;
    items: string[];
    tone?: 'default' | 'warn';
}

const BulletList: React.FC<BulletListProps> = ({ title, icon: Icon, items, tone = 'default' }) => {
    if (items.length === 0) return null;
    return (
        <div className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
            <div className="flex items-center gap-1.5 text-text/70">
                <Icon className="h-3.5 w-3.5" style={tone === 'warn' ? { color: 'var(--viz-warning)' } : undefined} />
                <MicroLabel>{title}</MicroLabel>
            </div>
            <ul className="mt-2 space-y-1.5">
                {items.map((t, i) => (
                    <li key={`${title}-${i}`} className="flex gap-2 text-xs leading-relaxed text-text">
                        <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-text/50" />
                        <span>{t}</span>
                    </li>
                ))}
            </ul>
        </div>
    );
};

/* ------------------------------------ main ---------------------------------- */

type BusyKey = 'start' | 'answer' | 'plan' | 'commit' | 'open';

interface BusyState {
    key: BusyKey;
    label: string;
    hint: string;
}

export const AIPlanner: React.FC<AIPlannerProps> = ({ onCommitted }) => {
    const { showToast } = useToast();

    const [status, setStatus] = useState<AIStatus | null>(null);
    const [statusLoading, setStatusLoading] = useState(true);
    const [statusError, setStatusError] = useState(false);

    const [session, setSession] = useState<PlanningSession | null>(null);
    const [busy, setBusy] = useState<BusyState | null>(null);

    // Setup form.
    const [rawText, setRawText] = useState('');
    const [notes, setNotes] = useState('');
    const [horizon, setHorizon] = useState(1);
    const [capacity, setCapacity] = useState('40');
    const [startDate, setStartDate] = useState(nextMonday);
    const [includeBacklog, setIncludeBacklog] = useState(false);
    const [lang, setLang] = useState<AILang>('uk');

    // Q & A.
    const [drafts, setDrafts] = useState<Record<string, string>>({});
    const [followUps, setFollowUps] = useState(0);
    const [showAnswers, setShowAnswers] = useState(false);

    // Sessions list.
    const [sessions, setSessions] = useState<PlanningSession[]>([]);
    const [sessionsLoading, setSessionsLoading] = useState(true);
    const [sessionsError, setSessionsError] = useState(false);
    const [sessionsOpen, setSessionsOpen] = useState(true);
    const [pendingDelete, setPendingDelete] = useState<string | null>(null);

    const [committed, setCommitted] = useState<{ sprint: Sprint; tasks: Task[] } | null>(null);

    const handleError = useCallback(
        (err: unknown, fallback: string) => {
            const e = asHttpError(err);
            const raw = serverMessage(err);
            // The API answers 503 both for "provider down" and "no key at all".
            if (e.response?.status === 503 && /GEMINI_API_KEY|disabled/i.test(raw)) {
                setStatus({ enabled: false, model: '' });
                return;
            }
            showToast(friendlyError(err, fallback), 'error');
        },
        [showToast]
    );

    const loadStatus = useCallback(async () => {
        try {
            const next = await getAIStatus();
            setStatus(next);
            setStatusError(false);
        } catch (err) {
            if (asHttpError(err).response?.status === 503) {
                setStatus({ enabled: false, model: '' });
                setStatusError(false);
            } else {
                setStatusError(true);
            }
        } finally {
            setStatusLoading(false);
        }
    }, []);

    const retryStatus = useCallback(() => {
        setStatusLoading(true);
        setStatusError(false);
        void loadStatus();
    }, [loadStatus]);

    const loadSessions = useCallback(async () => {
        try {
            const list = await listPlanningSessions(20);
            setSessions(Array.isArray(list) ? list : []);
            setSessionsError(false);
        } catch {
            setSessionsError(true);
        } finally {
            setSessionsLoading(false);
        }
    }, []);

    const retrySessions = useCallback(() => {
        setSessionsLoading(true);
        setSessionsError(false);
        void loadSessions();
    }, [loadSessions]);

    // Both fetches are kicked off a microtask late so the effect body itself never
    // touches state — the first paint is the skeleton, not a re-render cascade.
    useEffect(() => {
        void Promise.resolve().then(loadStatus);
    }, [loadStatus]);

    useEffect(() => {
        if (!status?.enabled) return;
        void Promise.resolve().then(loadSessions);
    }, [status?.enabled, loadSessions]);

    const taskLines = useMemo(
        () => rawText.split('\n').map((l) => l.trim()).filter((l) => l.length > 0),
        [rawText]
    );

    const items: PlanningItem[] = useMemo(() => session?.payload?.items ?? [], [session]);
    const questions: PlanningQuestion[] = useMemo(() => session?.payload?.questions ?? [], [session]);
    const plan = session?.payload?.plan;
    // The model occasionally omits load_percent — never render NaN%.
    const planLoadPct = plan ? Math.round(Number(plan.load_percent) || 0) : 0;

    const answeredCount = useMemo(() => questions.filter(isAnswered).length, [questions]);

    const pendingAnswers = useMemo(
        () =>
            questions
                .filter((q) => !isAnswered(q) && (drafts[q.id] ?? '').trim().length > 0)
                .map((q) => ({ question_id: q.id, answer: (drafts[q.id] ?? '').trim() })),
        [questions, drafts]
    );

    const groups = useMemo(() => {
        const byRef = new Map<string, PlanningQuestion[]>();
        for (const q of questions) {
            const bucket = byRef.get(q.item_ref);
            if (bucket) bucket.push(q);
            else byRef.set(q.item_ref, [q]);
        }
        const ordered: { ref: string; item: PlanningItem | null; questions: PlanningQuestion[] }[] = [];
        for (const item of items) {
            ordered.push({ ref: item.ref, item, questions: byRef.get(item.ref) ?? [] });
            byRef.delete(item.ref);
        }
        // Questions the planner attached to an item we never saw.
        for (const [ref, qs] of byRef) ordered.push({ ref, item: null, questions: qs });
        return ordered;
    }, [items, questions]);

    const titleByRef = useMemo(() => {
        const map: Record<string, string> = {};
        for (const item of items) map[item.ref] = item.title;
        for (const week of plan?.weeks ?? []) {
            for (const t of week.tasks ?? []) map[t.ref] = t.title;
        }
        return map;
    }, [items, plan]);

    const totalCapacity = session
        ? (session.capacity_hours_per_week || 0) * (session.horizon_weeks || 1)
        : 0;

    const itemsCommitted = useMemo(
        () =>
            round1(
                items.reduce((sum, it) => sum + (it.estimate_hours ?? 0) + (it.buffer_hours ?? 0), 0)
            ),
        [items]
    );

    const step = stepOf(session?.state);

    const resetToSetup = useCallback(() => {
        setSession(null);
        setDrafts({});
        setFollowUps(0);
        setCommitted(null);
        setShowAnswers(false);
        setSessionsOpen(true);
    }, []);

    const adoptSession = useCallback((next: PlanningSession) => {
        setSession(next);
        setDrafts({});
        setSessionsOpen(false);
    }, []);

    /* -------------------------------- actions -------------------------------- */

    const handleStart = async (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (busy) return;
        if (taskLines.length === 0) {
            showToast('Add at least one task — one per line', 'error');
            return;
        }
        const capacityNum = Number(capacity);
        if (!Number.isFinite(capacityNum) || capacityNum <= 0) {
            showToast('Capacity must be a positive number of hours', 'error');
            return;
        }

        setBusy({
            key: 'start',
            label: `Analysing ${taskLines.length} task${taskLines.length === 1 ? '' : 's'}...`,
            hint: 'The planner is working out what it still needs to ask you.',
        });
        setCommitted(null);
        setFollowUps(0);
        try {
            const next = await startPlanning({
                raw_tasks: taskLines,
                notes: notes.trim() || undefined,
                horizon_weeks: horizon,
                capacity_hours_per_week: capacityNum,
                starts_on: startDate || undefined,
                include_backlog: includeBacklog,
                lang,
            });
            adoptSession(next);
            showToast('Session started — answer the questions below', 'success');
            void loadSessions();
        } catch (err) {
            handleError(err, 'Could not start the planning session');
        } finally {
            setBusy(null);
        }
    };

    const handleAnswers = async () => {
        if (!session || busy || pendingAnswers.length === 0) return;

        const known = new Set(questions.map((q) => q.id));
        setBusy({
            key: 'answer',
            label: `Thinking through ${pendingAnswers.length} answer${pendingAnswers.length === 1 ? '' : 's'}...`,
            hint: 'Checking whether anything is still unclear before planning.',
        });
        try {
            const next = await answerPlanning(session.id, pendingAnswers);
            const nextQuestions = next.payload?.questions ?? [];
            const fresh = nextQuestions.filter((q) => !known.has(q.id) && !isAnswered(q)).length;

            setSession(next);
            setDrafts({});
            setFollowUps(next.state === 'COLLECTING' ? fresh : 0);

            if (next.state === 'READY') {
                showToast('Every item is resolved — review and generate the plan', 'success');
            } else if (fresh > 0) {
                showToast(`${fresh} follow-up question${fresh === 1 ? '' : 's'} added`, 'info');
            } else {
                showToast('Answers saved', 'success');
            }
            void loadSessions();
        } catch (err) {
            handleError(err, 'Could not save your answers');
        } finally {
            setBusy(null);
        }
    };

    const handleGenerate = async (regenerate = false) => {
        if (!session || busy) return;
        setBusy({
            key: 'plan',
            label: regenerate ? 'Rebuilding your sprint plan...' : 'Building your sprint plan...',
            hint: `Sequencing ${items.length} item${items.length === 1 ? '' : 's'} across ${session.horizon_weeks} week${session.horizon_weeks === 1 ? '' : 's'}.`,
        });
        try {
            const next = await generatePlan(session.id);
            setSession(next);
            setCommitted(null);
            showToast(regenerate ? 'Plan regenerated' : 'Your plan is ready', 'success');
            void loadSessions();
        } catch (err) {
            handleError(err, 'Could not generate the plan');
        } finally {
            setBusy(null);
        }
    };

    const handleCommit = async () => {
        if (!session || busy) return;
        const count = (plan?.weeks ?? []).reduce((n, w) => n + (w.tasks?.length ?? 0), 0);
        setBusy({
            key: 'commit',
            label: `Creating the sprint and ${count} task${count === 1 ? '' : 's'}...`,
            hint: 'Writing everything onto your board.',
        });
        try {
            const result = await commitPlan(session.id);
            setCommitted(result);
            setSession((prev) =>
                prev ? { ...prev, state: 'COMMITTED', sprint_id: result.sprint?.id } : prev
            );
            showToast('Added to your board', 'success');
            void loadSessions();
            onCommitted?.();
        } catch (err) {
            handleError(err, 'Could not add the plan to your board');
        } finally {
            setBusy(null);
        }
    };

    const handleOpenSession = async (id: string) => {
        if (busy) return;
        setBusy({ key: 'open', label: 'Opening the session...', hint: 'Fetching everything you answered.' });
        try {
            const next = await getPlanningSession(id);
            adoptSession(next);
            setCommitted(null);
            setFollowUps(0);
        } catch (err) {
            handleError(err, 'Could not open that session');
        } finally {
            setBusy(null);
        }
    };

    const handleDeleteSession = async (id: string) => {
        try {
            await deletePlanningSession(id);
            setSessions((prev) => prev.filter((s) => s.id !== id));
            if (session?.id === id) resetToSetup();
            showToast('Session deleted', 'success');
        } catch (err) {
            handleError(err, 'Could not delete that session');
        } finally {
            setPendingDelete(null);
        }
    };

    const setDraft = useCallback((id: string, value: string) => {
        setDrafts((prev) => ({ ...prev, [id]: value }));
    }, []);

    /* ------------------------------ gate renders ----------------------------- */

    if (statusLoading) {
        return (
            <div className="mx-auto w-full max-w-[1100px] px-4 py-6 sm:px-6">
                <div className="rounded-2xl border border-border bg-surface p-6 shadow-(--shadow)">
                    <div className="h-5 w-48 animate-pulse rounded-lg bg-hover" />
                    <div className="mt-4">
                        <SkeletonRows rows={4} />
                    </div>
                </div>
            </div>
        );
    }

    if (statusError) {
        return (
            <div className="mx-auto w-full max-w-[1100px] px-4 py-6 sm:px-6">
                <div className="rounded-2xl border border-border bg-surface p-6 text-center shadow-(--shadow)">
                    <TriangleAlert className="mx-auto h-5 w-5 text-danger" />
                    <h2 className="mt-3">Could not reach the AI service</h2>
                    <p className="text-sm text-text">
                        The planner could not check whether AI features are available.
                    </p>
                    <button
                        type="button"
                        onClick={retryStatus}
                        className="mt-4 inline-flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95"
                    >
                        <RefreshCw className="h-3.5 w-3.5" />
                        Try again
                    </button>
                </div>
            </div>
        );
    }

    if (!status?.enabled) {
        return (
            <div className="mx-auto w-full max-w-[1100px] px-4 py-6 sm:px-6">
                <div className="rounded-2xl border border-border bg-surface p-6 shadow-(--shadow) sm:p-8">
                    <div className="flex items-center gap-3">
                        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-hover text-text/70">
                            <Sparkles className="h-4 w-4" />
                        </div>
                        <MicroLabel>AI sprint planner</MicroLabel>
                    </div>

                    <h2 className="mt-4">The AI planner is switched off</h2>
                    <p className="max-w-prose text-sm leading-relaxed text-text">
                        This server has no Gemini key, so the planner cannot ask questions or build a
                        sprint. Set <code>GEMINI_API_KEY</code> in the backend environment and restart
                        it — everything else on the board keeps working meanwhile.
                    </p>

                    <button
                        type="button"
                        onClick={retryStatus}
                        className="mt-5 inline-flex items-center gap-2 rounded-xl border border-border bg-hover px-4 py-2 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                    >
                        <RefreshCw className="h-3.5 w-3.5" />
                        Check again
                    </button>
                </div>
            </div>
        );
    }

    /* -------------------------------- fragments ------------------------------ */

    const sessionsPanel = (
        <section className="rounded-2xl border border-border bg-surface shadow-(--shadow)">
            <button
                type="button"
                onClick={() => setSessionsOpen((v) => !v)}
                aria-expanded={sessionsOpen}
                className="flex w-full items-center justify-between gap-2 p-4 text-left transition hover:bg-hover"
            >
                <span className="flex items-center gap-2">
                    <History className="h-3.5 w-3.5 text-text/70" />
                    <MicroLabel>Previous sessions</MicroLabel>
                    {sessions.length > 0 && (
                        <Chip className="border-border bg-hover text-text">{sessions.length}</Chip>
                    )}
                </span>
                <ChevronDown
                    className={`h-3.5 w-3.5 shrink-0 text-text/60 transition-transform ${sessionsOpen ? 'rotate-180' : ''}`}
                />
            </button>

            {sessionsOpen && (
                <div className="border-t border-border/60 p-4">
                    {sessionsLoading ? (
                        <SkeletonRows rows={2} />
                    ) : sessionsError ? (
                        <div className="flex flex-col items-center gap-2 py-4 text-center">
                            <p className="text-xs text-text">Could not load your earlier sessions.</p>
                            <button
                                type="button"
                                onClick={retrySessions}
                                className="inline-flex items-center gap-1.5 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h transition active:scale-95"
                            >
                                <RefreshCw className="h-3.5 w-3.5" />
                                Retry
                            </button>
                        </div>
                    ) : sessions.length === 0 ? (
                        <p className="py-4 text-center text-xs text-text/70">
                            No planning sessions yet — the first one you start will show up here.
                        </p>
                    ) : (
                        <ul className="space-y-2">
                            {sessions.map((s) => {
                                const count = s.payload?.items?.length ?? 0;
                                const first = s.payload?.items?.[0]?.title;
                                return (
                                    <li
                                        key={s.id}
                                        className={`flex items-center gap-2 rounded-xl border p-2.5 transition ${
                                            session?.id === s.id
                                                ? 'border-accent-border bg-accent-bg'
                                                : 'border-border bg-bg hover:bg-hover'
                                        }`}
                                    >
                                        <button
                                            type="button"
                                            onClick={() => void handleOpenSession(s.id)}
                                            disabled={Boolean(busy)}
                                            className="flex min-w-0 flex-1 items-center gap-2 text-left disabled:opacity-50"
                                        >
                                            <span className="min-w-0 flex-1">
                                                <span className="flex flex-wrap items-center gap-1.5">
                                                    <Chip className={STATE_CHIP[s.state]}>
                                                        {STATE_LABEL[s.state]}
                                                    </Chip>
                                                    <span className="font-mono text-[10px] uppercase tracking-wider text-text/60">
                                                        {formatStamp(s.created_at)}
                                                    </span>
                                                </span>
                                                <span className="mt-1 block truncate text-xs text-text-h">
                                                    {first ?? 'Untitled session'}
                                                    {count > 1 ? ` +${count - 1} more` : ''}
                                                </span>
                                            </span>
                                            <ChevronRight className="h-3.5 w-3.5 shrink-0 text-text/50" />
                                        </button>

                                        {pendingDelete === s.id ? (
                                            <span className="flex shrink-0 items-center gap-1">
                                                <button
                                                    type="button"
                                                    onClick={() => void handleDeleteSession(s.id)}
                                                    className="rounded-lg bg-danger px-2 py-1 text-[10px] font-bold uppercase tracking-wider text-bg transition active:scale-95"
                                                >
                                                    Delete
                                                </button>
                                                <button
                                                    type="button"
                                                    onClick={() => setPendingDelete(null)}
                                                    className="rounded-lg border border-border px-2 py-1 text-[10px] font-semibold uppercase tracking-wider text-text transition active:scale-95"
                                                >
                                                    No
                                                </button>
                                            </span>
                                        ) : (
                                            <button
                                                type="button"
                                                onClick={() => setPendingDelete(s.id)}
                                                title="Delete session"
                                                aria-label="Delete session"
                                                className="shrink-0 rounded-lg p-1.5 text-text/60 transition hover:bg-danger/10 hover:text-danger active:scale-95"
                                            >
                                                <Trash2 className="h-3.5 w-3.5" />
                                            </button>
                                        )}
                                    </li>
                                );
                            })}
                        </ul>
                    )}
                </div>
            )}
        </section>
    );

    const answeredList = questions.filter(isAnswered);

    /* --------------------------------- render -------------------------------- */

    return (
        <div className="mx-auto w-full max-w-[1100px] px-4 py-6 sm:px-6">
            {/* Header */}
            <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="flex min-w-0 items-center gap-3">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-accent-border bg-accent-bg text-accent">
                        <Sparkles className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                        <h2>AI sprint planner</h2>
                        <p className="text-xs text-text">
                            List what you need to do, answer a few questions, get a week-by-week plan.
                        </p>
                    </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                    {status.model && (
                        <Chip className="border-border bg-hover text-text">{status.model}</Chip>
                    )}
                    {session && (
                        <button
                            type="button"
                            onClick={resetToSetup}
                            disabled={Boolean(busy)}
                            className="inline-flex items-center gap-1.5 rounded-xl border border-border bg-hover px-3 py-1.5 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95 disabled:opacity-50"
                        >
                            <ArrowLeft className="h-3.5 w-3.5" />
                            New session
                        </button>
                    )}
                </div>
            </div>

            {/* Stepper */}
            <ol className="mt-5 flex items-center gap-1.5 overflow-x-auto no-scrollbar">
                {STEPS.map((label, i) => {
                    const state = i === step ? 'current' : i < step ? 'done' : 'todo';
                    return (
                        <li key={label} className="flex shrink-0 items-center gap-1.5">
                            <span
                                className={`inline-flex items-center gap-1.5 rounded-xl border px-2.5 py-1 text-[10px] font-mono font-medium uppercase tracking-wider transition ${
                                    state === 'current'
                                        ? 'border-transparent bg-text-h text-bg'
                                        : state === 'done'
                                          ? 'border-success/30 bg-success/10 text-success'
                                          : 'border-border bg-hover text-text/60'
                                }`}
                            >
                                {state === 'done' ? (
                                    <Check className="h-3 w-3" strokeWidth={3} />
                                ) : (
                                    <span>{i + 1}</span>
                                )}
                                {label}
                            </span>
                            {i < STEPS.length - 1 && (
                                <span className="h-px w-4 shrink-0 bg-border sm:w-6" aria-hidden="true" />
                            )}
                        </li>
                    );
                })}
            </ol>

            {/* Session meta strip */}
            {session && (
                <div className="mt-3 flex flex-wrap items-center gap-1.5">
                    <Chip className={STATE_CHIP[session.state]}>{STATE_LABEL[session.state]}</Chip>
                    <Chip className="border-border bg-hover text-text">
                        <Layers className="h-3 w-3" />
                        {items.length} item{items.length === 1 ? '' : 's'}
                    </Chip>
                    <Chip className="border-border bg-hover text-text">
                        <CalendarDays className="h-3 w-3" />
                        {session.horizon_weeks} week{session.horizon_weeks === 1 ? '' : 's'}
                    </Chip>
                    <Chip className="border-border bg-hover text-text">
                        <Gauge className="h-3 w-3" />
                        {session.capacity_hours_per_week}h / week
                    </Chip>
                    {session.starts_on && (
                        <Chip className="border-border bg-hover text-text">
                            From {formatDay(session.starts_on)}
                        </Chip>
                    )}
                </div>
            )}

            <div className="mt-4 space-y-4">
                {/* -------------------------- Step 1 — setup ------------------------- */}
                {!session && (
                    <form
                        onSubmit={handleStart}
                        className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) sm:p-5"
                    >
                        <div className="flex items-center justify-between gap-2">
                            <MicroLabel>What do you need to do?</MicroLabel>
                            <span className="font-mono text-[10px] uppercase tracking-wider text-text/70">
                                {taskLines.length} task{taskLines.length === 1 ? '' : 's'}
                            </span>
                        </div>

                        <textarea
                            value={rawText}
                            onChange={(e) => setRawText(e.target.value)}
                            disabled={Boolean(busy)}
                            rows={7}
                            placeholder={'One task per line, for example:\nShip the billing webhook\nRewrite the onboarding email\nFix the flaky auth test'}
                            className="mt-2 w-full resize-y rounded-xl border border-border bg-bg px-3 py-2.5 font-mono text-xs leading-relaxed text-text-h outline-none transition placeholder:text-text/50 focus:border-accent disabled:opacity-50"
                        />

                        <div className="mt-3">
                            <MicroLabel>Notes for the planner</MicroLabel>
                            <textarea
                                value={notes}
                                onChange={(e) => setNotes(e.target.value)}
                                disabled={Boolean(busy)}
                                rows={2}
                                placeholder="Anything it should know — deadlines, people, days off..."
                                className="mt-1.5 w-full resize-y rounded-xl border border-border bg-bg px-3 py-2 text-xs text-text-h outline-none transition placeholder:text-text/50 focus:border-accent disabled:opacity-50"
                            />
                        </div>

                        <div className="mt-4 grid gap-3 sm:grid-cols-2">
                            <div>
                                <MicroLabel>Horizon</MicroLabel>
                                <div className="mt-1.5 inline-flex rounded-xl border border-border bg-bg p-1">
                                    {[1, 2].map((w) => (
                                        <button
                                            key={w}
                                            type="button"
                                            onClick={() => setHorizon(w)}
                                            disabled={Boolean(busy)}
                                            aria-pressed={horizon === w}
                                            className={`rounded-lg px-3 py-1 text-xs font-semibold transition active:scale-95 disabled:opacity-50 ${
                                                horizon === w
                                                    ? 'bg-text-h text-bg'
                                                    : 'text-text hover:text-text-h'
                                            }`}
                                        >
                                            {w} week{w === 1 ? '' : 's'}
                                        </button>
                                    ))}
                                </div>
                            </div>

                            <div>
                                <MicroLabel>Language</MicroLabel>
                                <div className="mt-1.5 inline-flex rounded-xl border border-border bg-bg p-1">
                                    {(['uk', 'en'] as AILang[]).map((l) => (
                                        <button
                                            key={l}
                                            type="button"
                                            onClick={() => setLang(l)}
                                            disabled={Boolean(busy)}
                                            aria-pressed={lang === l}
                                            className={`rounded-lg px-3 py-1 font-mono text-xs font-semibold uppercase tracking-wider transition active:scale-95 disabled:opacity-50 ${
                                                lang === l ? 'bg-text-h text-bg' : 'text-text hover:text-text-h'
                                            }`}
                                        >
                                            {l}
                                        </button>
                                    ))}
                                </div>
                            </div>

                            <div>
                                <MicroLabel>Capacity, hours per week</MicroLabel>
                                <input
                                    type="number"
                                    min={1}
                                    max={200}
                                    step={1}
                                    inputMode="numeric"
                                    value={capacity}
                                    onChange={(e) => setCapacity(e.target.value)}
                                    disabled={Boolean(busy)}
                                    className="mt-1.5 w-full rounded-xl border border-border bg-bg px-3 py-2 text-sm text-text-h outline-none transition focus:border-accent disabled:opacity-50"
                                />
                            </div>

                            <div>
                                <MicroLabel>Starts on</MicroLabel>
                                <input
                                    type="date"
                                    value={startDate}
                                    onChange={(e) => setStartDate(e.target.value)}
                                    disabled={Boolean(busy)}
                                    className="mt-1.5 w-full rounded-xl border border-border bg-bg px-3 py-2 text-sm text-text-h outline-none transition focus:border-accent disabled:opacity-50"
                                />
                            </div>
                        </div>

                        <button
                            type="button"
                            role="checkbox"
                            aria-checked={includeBacklog}
                            onClick={() => setIncludeBacklog((v) => !v)}
                            disabled={Boolean(busy)}
                            className="mt-4 flex items-center gap-2 rounded-xl px-1 py-1 text-left transition active:scale-95 disabled:opacity-50"
                        >
                            <span
                                className={`flex h-4 w-4 shrink-0 items-center justify-center rounded-md border transition ${
                                    includeBacklog
                                        ? 'border-success bg-success text-bg'
                                        : 'border-border bg-bg'
                                }`}
                            >
                                {includeBacklog && <Check className="h-3 w-3" strokeWidth={3} />}
                            </span>
                            <span className="text-xs text-text-h">Include my open TODOs</span>
                        </button>

                        <div className="mt-4 border-t border-border/60 pt-4">
                            {busy?.key === 'start' ? (
                                <BusyPanel key={busy.key} label={busy.label} hint={busy.hint} />
                            ) : (
                                <div className="flex flex-wrap items-center justify-between gap-3">
                                    <p className="text-xs text-text/70">
                                        The planner will ask about blockers, effort and buffer for each task.
                                    </p>
                                    <button
                                        type="submit"
                                        disabled={Boolean(busy) || taskLines.length === 0}
                                        className="inline-flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                                    >
                                        <Sparkles className="h-3.5 w-3.5" />
                                        Start planning
                                    </button>
                                </div>
                            )}
                        </div>
                    </form>
                )}

                {/* -------------------------- Step 2 — Q & A ------------------------- */}
                {session?.state === 'COLLECTING' && (
                    <>
                        <div className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
                            <div className="flex flex-wrap items-center justify-between gap-2">
                                <span className="flex items-center gap-2">
                                    <ListChecks className="h-4 w-4 text-accent" />
                                    <MicroLabel>Progress</MicroLabel>
                                </span>
                                <span className="font-mono text-xs text-text-h">
                                    {answeredCount} of {questions.length} answered
                                </span>
                            </div>
                            <div className="mt-2">
                                <LoadBar
                                    value={answeredCount}
                                    max={questions.length}
                                    label="Questions answered"
                                />
                            </div>
                            {followUps > 0 && (
                                <p className="mt-3 flex items-start gap-2 rounded-xl border border-info/30 bg-info/10 p-2.5 text-xs text-info">
                                    <Info className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                                    <span>
                                        The planner needs a bit more: {followUps} follow-up question
                                        {followUps === 1 ? '' : 's'} {followUps === 1 ? 'was' : 'were'} added
                                        below.
                                    </span>
                                </p>
                            )}
                        </div>

                        {questions.length === 0 ? (
                            <div className="rounded-2xl border border-border bg-surface p-6 text-center shadow-(--shadow)">
                                <MessageSquare className="mx-auto h-5 w-5 text-text/50" />
                                <p className="mt-3 text-sm text-text-h">No questions came back.</p>
                                <p className="mt-1 text-xs text-text">
                                    The planner is still collecting — reopen the session in a moment, or
                                    start a new one.
                                </p>
                            </div>
                        ) : (
                            groups.map((group) => {
                                const pending = group.questions.filter((q) => !isAnswered(q)).length;
                                if (group.questions.length === 0 && !group.item) return null;
                                return (
                                    <section
                                        key={group.ref}
                                        className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)"
                                    >
                                        <div className="flex flex-wrap items-start justify-between gap-2">
                                            <div className="min-w-0">
                                                <MicroLabel>Task {group.ref}</MicroLabel>
                                                <h4 className="mt-1 text-sm font-semibold leading-snug text-text-h">
                                                    {group.item?.title ?? 'Unmatched item'}
                                                </h4>
                                                {group.item?.notes && (
                                                    <p className="mt-1 text-xs text-text">{group.item.notes}</p>
                                                )}
                                            </div>
                                            <div className="flex shrink-0 flex-wrap items-center gap-1.5">
                                                {group.item?.priority && (
                                                    <PriorityChip priority={group.item.priority} />
                                                )}
                                                {group.item?.resolved ? (
                                                    <Chip className="border-success/30 bg-success/10 text-success">
                                                        <Check className="h-3 w-3" strokeWidth={3} />
                                                        Resolved
                                                    </Chip>
                                                ) : (
                                                    <Chip className="border-border bg-hover text-text">
                                                        {pending} open
                                                    </Chip>
                                                )}
                                            </div>
                                        </div>

                                        {group.questions.length === 0 ? (
                                            <p className="mt-3 text-xs text-text/70">
                                                Nothing left to ask about this one.
                                            </p>
                                        ) : (
                                            <div className="mt-3 space-y-2">
                                                {group.questions.map((q) => (
                                                    <QuestionBlock
                                                        key={q.id}
                                                        question={q}
                                                        draft={drafts[q.id] ?? ''}
                                                        disabled={Boolean(busy)}
                                                        onDraft={setDraft}
                                                    />
                                                ))}
                                            </div>
                                        )}
                                    </section>
                                );
                            })
                        )}

                        <div className="sticky bottom-3 z-10">
                            {busy?.key === 'answer' ? (
                                <BusyPanel key={busy.key} label={busy.label} hint={busy.hint} />
                            ) : (
                                <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-surface/95 p-3 shadow-(--shadow) backdrop-blur-xl">
                                    <p className="text-xs text-text/70">
                                        Answer as many as you like, then send them together.
                                    </p>
                                    <button
                                        type="button"
                                        onClick={() => void handleAnswers()}
                                        disabled={Boolean(busy) || pendingAnswers.length === 0}
                                        className="inline-flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                                    >
                                        <Send className="h-3.5 w-3.5" />
                                        Submit {pendingAnswers.length > 0 ? `${pendingAnswers.length} ` : ''}
                                        answer{pendingAnswers.length === 1 ? '' : 's'}
                                    </button>
                                </div>
                            )}
                        </div>
                    </>
                )}

                {/* ------------------------- Step 3 — review ------------------------- */}
                {session?.state === 'READY' && (
                    <>
                        <section className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
                            <div className="flex flex-wrap items-center justify-between gap-2">
                                <span className="flex items-center gap-2">
                                    <Target className="h-4 w-4 text-accent" />
                                    <MicroLabel>Everything the planner knows</MicroLabel>
                                </span>
                                <span
                                    className="font-mono text-xs text-text-h"
                                    style={
                                        itemsCommitted > totalCapacity && totalCapacity > 0
                                            ? { color: 'var(--viz-warning)' }
                                            : undefined
                                    }
                                >
                                    {formatHours(itemsCommitted)} of {formatHours(totalCapacity)}
                                </span>
                            </div>

                            <div className="mt-2">
                                <LoadBar value={itemsCommitted} max={totalCapacity} label="Committed hours" />
                            </div>

                            {totalCapacity > 0 && itemsCommitted > totalCapacity && (
                                <p
                                    className="mt-2 flex items-start gap-1.5 text-xs"
                                    style={{ color: 'var(--viz-warning)' }}
                                >
                                    <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                                    <span>
                                        That is {formatHours(itemsCommitted - totalCapacity)} over your
                                        capacity — expect the planner to defer something.
                                    </span>
                                </p>
                            )}

                            {items.length === 0 ? (
                                <p className="mt-4 rounded-xl border border-dashed border-border p-6 text-center text-xs text-text/70">
                                    This session has no items.
                                </p>
                            ) : (
                                <div className="mt-3 -mx-4 overflow-x-auto px-4">
                                    <table className="w-full min-w-[720px] border-collapse text-left">
                                        <thead>
                                            <tr className="border-b border-border">
                                                {['Task', 'Priority', 'Estimate', 'Buffer', 'Blockers', 'Reviewer'].map(
                                                    (h) => (
                                                        <th key={h} className="px-2 py-2">
                                                            <MicroLabel>{h}</MicroLabel>
                                                        </th>
                                                    )
                                                )}
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {items.map((it) => (
                                                <tr
                                                    key={it.ref}
                                                    className="border-b border-border/60 align-top last:border-0"
                                                >
                                                    <td className="max-w-[280px] px-2 py-2.5">
                                                        <span className="block text-xs font-medium text-text-h">
                                                            {it.title}
                                                        </span>
                                                        {it.depends_on && it.depends_on.length > 0 && (
                                                            <span className="mt-1 block font-mono text-[10px] text-text/60">
                                                                after{' '}
                                                                {it.depends_on
                                                                    .map((r) => titleByRef[r] ?? r)
                                                                    .join(', ')}
                                                            </span>
                                                        )}
                                                    </td>
                                                    <td className="px-2 py-2.5">
                                                        <PriorityChip priority={it.priority} />
                                                    </td>
                                                    <td className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-text-h">
                                                        {formatHours(it.estimate_hours)}
                                                    </td>
                                                    <td className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-text">
                                                        {formatHours(it.buffer_hours)}
                                                    </td>
                                                    <td className="max-w-[220px] px-2 py-2.5 text-xs text-text">
                                                        {it.has_blockers || it.blockers ? (
                                                            <span style={{ color: 'var(--viz-warning)' }}>
                                                                {it.blockers || 'Yes'}
                                                            </span>
                                                        ) : (
                                                            <span className="text-text/60">None</span>
                                                        )}
                                                    </td>
                                                    <td className="px-2 py-2.5 text-xs text-text">
                                                        {it.needs_review ? (
                                                            it.reviewer || 'Needs review'
                                                        ) : (
                                                            <span className="text-text/60">—</span>
                                                        )}
                                                    </td>
                                                </tr>
                                            ))}
                                        </tbody>
                                    </table>
                                </div>
                            )}
                        </section>

                        {answeredList.length > 0 && (
                            <section className="rounded-2xl border border-border bg-surface shadow-(--shadow)">
                                <button
                                    type="button"
                                    onClick={() => setShowAnswers((v) => !v)}
                                    aria-expanded={showAnswers}
                                    className="flex w-full items-center justify-between gap-2 p-4 text-left transition hover:bg-hover"
                                >
                                    <span className="flex items-center gap-2">
                                        <MessageSquare className="h-3.5 w-3.5 text-text/70" />
                                        <MicroLabel>What you answered ({answeredList.length})</MicroLabel>
                                    </span>
                                    <ChevronDown
                                        className={`h-3.5 w-3.5 text-text/60 transition-transform ${showAnswers ? 'rotate-180' : ''}`}
                                    />
                                </button>
                                {showAnswers && (
                                    <div className="space-y-2 border-t border-border/60 p-4">
                                        {answeredList.map((q) => (
                                            <QuestionBlock
                                                key={q.id}
                                                question={q}
                                                draft=""
                                                disabled
                                                onDraft={setDraft}
                                            />
                                        ))}
                                    </div>
                                )}
                            </section>
                        )}

                        {busy?.key === 'plan' ? (
                            <BusyPanel key={busy.key} label={busy.label} hint={busy.hint} />
                        ) : (
                            <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
                                <p className="text-xs text-text/70">
                                    Ready when you are — this usually takes 10 to 30 seconds.
                                </p>
                                <button
                                    type="button"
                                    onClick={() => void handleGenerate(false)}
                                    disabled={Boolean(busy)}
                                    className="inline-flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                                >
                                    <Zap className="h-3.5 w-3.5" />
                                    Generate the plan
                                </button>
                            </div>
                        )}
                    </>
                )}

                {/* -------------------------- Step 4 — plan -------------------------- */}
                {(session?.state === 'PLANNED' || session?.state === 'COMMITTED') && (
                    <>
                        {!plan ? (
                            busy?.key === 'plan' ? (
                                <BusyPanel key={busy.key} label={busy.label} hint={busy.hint} />
                            ) : (
                                <div className="rounded-2xl border border-border bg-surface p-6 text-center shadow-(--shadow)">
                                    <TriangleAlert className="mx-auto h-5 w-5 text-warning" />
                                    <p className="mt-3 text-sm text-text-h">
                                        This session has no plan attached.
                                    </p>
                                    <p className="mt-1 text-xs text-text">
                                        Generating it again usually fixes that.
                                    </p>
                                    <button
                                        type="button"
                                        onClick={() => void handleGenerate(true)}
                                        disabled={Boolean(busy)}
                                        className="mt-4 inline-flex items-center gap-2 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                                    >
                                        <RotateCcw className="h-3.5 w-3.5" />
                                        Generate again
                                    </button>
                                </div>
                            )
                        ) : (
                            <>
                                <section className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) sm:p-5">
                                    <div className="flex flex-wrap items-start justify-between gap-3">
                                        <div className="min-w-0">
                                            <MicroLabel>Sprint</MicroLabel>
                                            <h3 className="mt-1 text-lg font-semibold leading-snug text-text-h">
                                                {plan.name}
                                            </h3>
                                            {plan.goal && (
                                                <p className="mt-1 max-w-prose text-sm leading-relaxed text-text">
                                                    {plan.goal}
                                                </p>
                                            )}
                                        </div>
                                        <Chip className="border-border bg-hover text-text">
                                            <CalendarDays className="h-3 w-3" />
                                            {formatRange(plan.starts_on, plan.ends_on)}
                                        </Chip>
                                    </div>

                                    <div className="mt-4">
                                        <div className="flex flex-wrap items-center justify-between gap-2">
                                            <MicroLabel>Committed vs capacity</MicroLabel>
                                            <span
                                                className="font-mono text-xs text-text-h"
                                                style={
                                                    planLoadPct > 100
                                                        ? { color: 'var(--viz-warning)' }
                                                        : undefined
                                                }
                                            >
                                                {formatHours(plan.committed_hours)} /{' '}
                                                {formatHours(plan.capacity_hours)} ·{' '}
                                                {planLoadPct}%
                                            </span>
                                        </div>
                                        <div className="mt-2">
                                            <LoadBar
                                                value={plan.committed_hours}
                                                max={plan.capacity_hours}
                                                label="Sprint load"
                                            />
                                        </div>
                                    </div>

                                    <div className="mt-4 grid grid-cols-2 gap-2.5 lg:grid-cols-4">
                                        <StatTile
                                            label="Estimate"
                                            value={formatHours(plan.total_estimate_hours)}
                                            icon={Clock}
                                        />
                                        <StatTile
                                            label="Buffer"
                                            value={formatHours(plan.total_buffer_hours)}
                                            icon={ShieldAlert}
                                        />
                                        <StatTile
                                            label="Committed"
                                            value={formatHours(plan.committed_hours)}
                                            icon={Target}
                                        />
                                        <StatTile
                                            label="Load"
                                            value={`${planLoadPct}%`}
                                            icon={Gauge}
                                            tone={planLoadPct > 100 ? 'warn' : 'default'}
                                        />
                                    </div>
                                </section>

                                {(plan.weeks ?? []).length === 0 ? (
                                    <div className="rounded-2xl border border-dashed border-border p-6 text-center text-xs text-text/70">
                                        The plan came back with no weeks in it.
                                    </div>
                                ) : (
                                    (plan.weeks ?? []).map((w) => (
                                        <WeekCard key={w.index} week={w} titleByRef={titleByRef} />
                                    ))
                                )}

                                {(plan.deferred ?? []).length > 0 && (
                                    <section className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
                                        <div className="flex items-center gap-1.5 text-text/70">
                                            <Clock className="h-3.5 w-3.5" />
                                            <MicroLabel>Deferred ({(plan.deferred ?? []).length})</MicroLabel>
                                        </div>
                                        <ul className="mt-2 space-y-2">
                                            {(plan.deferred ?? []).map((d) => (
                                                <li
                                                    key={d.ref}
                                                    className="rounded-xl border border-border bg-bg p-3"
                                                >
                                                    <p className="text-xs font-medium text-text-h">{d.title}</p>
                                                    <p className="mt-1 text-xs text-text">{d.reason}</p>
                                                </li>
                                            ))}
                                        </ul>
                                    </section>
                                )}

                                <BulletList
                                    title="Risks"
                                    icon={TriangleAlert}
                                    items={plan.risks ?? []}
                                    tone="warn"
                                />
                                <BulletList
                                    title="Recommendations"
                                    icon={Lightbulb}
                                    items={plan.recommendations ?? []}
                                />

                                {plan.rationale && (
                                    <section className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
                                        <div className="flex items-center gap-1.5 text-text/70">
                                            <Info className="h-3.5 w-3.5" />
                                            <MicroLabel>Why this plan</MicroLabel>
                                        </div>
                                        <p className="mt-2 rounded-xl bg-code-bg p-3 text-xs leading-relaxed text-text-h">
                                            {plan.rationale}
                                        </p>
                                    </section>
                                )}

                                {committed ? (
                                    <section className="rounded-2xl border border-success/30 bg-success/10 p-4 shadow-(--shadow)">
                                        <div className="flex items-start gap-3">
                                            <CheckCircle2 className="mt-0.5 h-4 w-4 shrink-0 text-success" />
                                            <div className="min-w-0">
                                                <p className="text-sm font-semibold text-text-h">
                                                    {committed.sprint?.name ?? plan.name} is on your board
                                                </p>
                                                <p className="mt-1 text-xs text-text">
                                                    {(committed.tasks ?? []).length} task
                                                    {(committed.tasks ?? []).length === 1 ? '' : 's'} created ·{' '}
                                                    {formatRange(
                                                        committed.sprint?.starts_on,
                                                        committed.sprint?.ends_on
                                                    )}
                                                </p>
                                            </div>
                                        </div>
                                    </section>
                                ) : session.state === 'COMMITTED' ? (
                                    <section className="rounded-2xl border border-success/30 bg-success/10 p-4">
                                        <p className="flex items-center gap-2 text-xs text-text-h">
                                            <CheckCircle2 className="h-4 w-4 shrink-0 text-success" />
                                            This plan has already been added to your board.
                                        </p>
                                    </section>
                                ) : busy?.key === 'commit' || busy?.key === 'plan' ? (
                                    <BusyPanel key={busy.key} label={busy.label} hint={busy.hint} />
                                ) : (
                                    <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)">
                                        <button
                                            type="button"
                                            onClick={() => void handleGenerate(true)}
                                            disabled={Boolean(busy)}
                                            className="inline-flex items-center gap-2 rounded-xl border border-border bg-hover px-3 py-2 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95 disabled:opacity-50"
                                        >
                                            <RotateCcw className="h-3.5 w-3.5" />
                                            Regenerate
                                        </button>
                                        <button
                                            type="button"
                                            onClick={() => void handleCommit()}
                                            disabled={Boolean(busy)}
                                            className="inline-flex items-center gap-2 rounded-xl bg-success px-5 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                                        >
                                            <Plus className="h-3.5 w-3.5" strokeWidth={3} />
                                            Add to my board
                                        </button>
                                    </div>
                                )}
                            </>
                        )}
                    </>
                )}

                {busy?.key === 'open' && (
                    <BusyPanel key={busy.key} label={busy.label} hint={busy.hint} />
                )}

                {sessionsPanel}
            </div>
        </div>
    );
};

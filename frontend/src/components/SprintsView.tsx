import React, { useCallback, useEffect, useMemo, useState } from 'react';
import {
    Archive,
    CalendarClock,
    CalendarRange,
    ChevronDown,
    CircleCheckBig,
    Clock,
    Gauge,
    Layers,
    ListChecks,
    LoaderCircle,
    Pencil,
    Plus,
    RefreshCw,
    Sparkles,
    Target,
    Timer,
    Trash2,
    TriangleAlert,
    User,
    X,
} from 'lucide-react';
import { createSprint, deleteSprint, getSprint, listSprints, updateSprint } from '../api/endpoints';
import type { SprintInput } from '../api/endpoints';
import { BOARD_STATUSES, PRIORITY_LABELS, STATUS_LABELS } from '../types';
import type { Sprint, SprintStats, SprintStatus, Task, TaskPriority } from '../types';
import { useToast } from '../context/ToastContext';

interface SprintsViewProps {
    onOpenTask?: (taskId: string) => void;
}

/* --------------------------------- helpers ---------------------------------- */

const SPRINT_STATUSES: SprintStatus[] = ['PLANNED', 'ACTIVE', 'COMPLETED', 'ARCHIVED'];

type IconType = React.ComponentType<{ className?: string }>;

/**
 * Reserved status tokens only — these never double as a series colour.
 * PLANNED and ARCHIVED share the muted token and are told apart by their icon.
 */
const STATUS_META: Record<SprintStatus, { token: string; icon: IconType }> = {
    PLANNED: { token: 'var(--viz-muted)', icon: CalendarClock },
    ACTIVE: { token: 'var(--viz-warning)', icon: Clock },
    COMPLETED: { token: 'var(--viz-good)', icon: CircleCheckBig },
    ARCHIVED: { token: 'var(--viz-muted)', icon: Archive },
};

/** Sort order for the board: what is running first, what is filed away last. */
const STATUS_RANK: Record<SprintStatus, number> = {
    ACTIVE: 0,
    PLANNED: 1,
    COMPLETED: 2,
    ARCHIVED: 3,
};

/** Ordinal ramp, LOW -> URGENT. Carried by a dot so the label keeps ink contrast. */
const PRIORITY_TOKEN: Record<TaskPriority, string> = {
    LOW: 'var(--viz-ord-1)',
    MEDIUM: 'var(--viz-ord-2)',
    HIGH: 'var(--viz-ord-3)',
    URGENT: 'var(--viz-ord-4)',
};

/** Mixing the token toward the ink token keeps small text legible in both themes. */
const tokenChip = (token: string): React.CSSProperties => ({
    color: `color-mix(in oklab, ${token} 80%, var(--text-h))`,
    borderColor: `color-mix(in oklab, ${token} 40%, transparent)`,
    backgroundColor: `color-mix(in oklab, ${token} 12%, transparent)`,
});

const errorMessage = (err: unknown, fallback: string): string => {
    if (typeof err === 'object' && err !== null) {
        const data = (err as { response?: { data?: { error?: unknown } } }).response?.data;
        if (data && typeof data.error === 'string' && data.error.trim()) return data.error;
    }
    return fallback;
};

/** The API sends either `YYYY-MM-DD` or a full RFC3339 stamp. */
const toDateInput = (value?: string): string => (value ? value.slice(0, 10) : '');

const parseDay = (value?: string): Date | null => {
    const day = toDateInput(value);
    if (!day) return null;
    const parsed = new Date(`${day}T00:00:00`);
    return Number.isNaN(parsed.getTime()) ? null : parsed;
};

const formatDay = (value?: string, withYear = false): string => {
    const parsed = parseDay(value);
    if (!parsed) return '—';
    return parsed.toLocaleDateString(undefined, {
        day: '2-digit',
        month: 'short',
        ...(withYear ? { year: 'numeric' } : {}),
    });
};

const spanDays = (from?: string, to?: string): number | null => {
    const a = parseDay(from);
    const b = parseDay(to);
    if (!a || !b) return null;
    return Math.round((b.getTime() - a.getTime()) / 86400000) + 1;
};

const formatHours = (value?: number): string => {
    if (value === undefined || value === null || Number.isNaN(value)) return '—';
    return `${Number.isInteger(value) ? value : value.toFixed(1)}h`;
};

const clamp01 = (value: number): number => Math.min(1, Math.max(0, value));

/* ------------------------------- small pieces -------------------------------- */

const Stat: React.FC<{ icon: IconType; label: string; value: string; hint?: string }> = ({
    icon: Icon,
    label,
    value,
    hint,
}) => (
    <div className="rounded-xl border border-border bg-bg px-3 py-2">
        <span className="flex items-center gap-1.5 text-[10px] font-mono uppercase tracking-wider text-text">
            <Icon className="h-3.5 w-3.5 shrink-0 text-text/70" />
            <span className="truncate">{label}</span>
        </span>
        <span className="mt-1 block text-sm font-semibold leading-none text-text-h">{value}</span>
        {hint && <span className="mt-1 block text-[10px] leading-none text-text/70">{hint}</span>}
    </div>
);

interface MeterProps {
    label: string;
    valueLabel: string;
    /** Filled portion of the track, 0..1. */
    ratio: number;
    fill: string;
    track: string;
    /** Optional reference mark (capacity), 0..1. */
    tick?: number;
    note?: string;
}

const Meter: React.FC<MeterProps> = ({ label, valueLabel, ratio, fill, track, tick, note }) => (
    <div className="min-w-0">
        <div className="flex items-baseline justify-between gap-2">
            <span className="truncate text-[10px] font-mono uppercase tracking-wider text-text">
                {label}
            </span>
            <span className="shrink-0 text-xs font-semibold tabular-nums text-text-h">
                {valueLabel}
            </span>
        </div>
        <div
            aria-hidden="true"
            className="relative mt-1.5 h-2 w-full overflow-hidden rounded-full"
            style={{ background: track }}
        >
            <div
                className="h-full rounded-full transition-[width] duration-500"
                style={{ width: `${clamp01(ratio) * 100}%`, background: fill }}
            />
            {tick !== undefined && tick < 1 && (
                <span
                    className="absolute inset-y-0 w-0.5"
                    style={{ left: `${clamp01(tick) * 100}%`, background: 'var(--surface)' }}
                />
            )}
        </div>
        {note && <p className="mt-1 text-[10px] leading-none text-text/70">{note}</p>}
    </div>
);

const StatusChip: React.FC<{ status: SprintStatus }> = ({ status }) => {
    const meta = STATUS_META[status];
    const Icon = meta.icon;
    return (
        <span
            className="inline-flex shrink-0 items-center gap-1.5 rounded-lg border px-2.5 py-0.5 text-[10px] font-mono font-medium uppercase tracking-wider"
            style={tokenChip(meta.token)}
        >
            <Icon className="h-3.5 w-3.5" />
            {status}
        </span>
    );
};

/* ---------------------------------- modals ----------------------------------- */

const ModalShell: React.FC<{
    icon: IconType;
    title: string;
    onClose: () => void;
    children: React.ReactNode;
}> = ({ icon: Icon, title, onClose, children }) => {
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose]);

    return (
        <div
            role="dialog"
            aria-modal="true"
            aria-label={title}
            className="fixed inset-0 z-50 flex items-end justify-center bg-black/60 p-0 backdrop-blur-md sm:items-center sm:p-4"
            onMouseDown={(e) => {
                if (e.target === e.currentTarget) onClose();
            }}
        >
            <div className="max-h-[92svh] w-full max-w-lg overflow-y-auto rounded-t-3xl border border-border bg-surface p-5 shadow-(--shadow) sm:rounded-2xl sm:p-6">
                <div className="flex items-center justify-between gap-3 border-b border-border pb-4">
                    <div className="flex min-w-0 items-center gap-2.5">
                        <div className="rounded-xl border border-border bg-code-bg p-2 text-success">
                            <Icon className="h-4 w-4" />
                        </div>
                        <h2 className="truncate font-mono text-sm font-bold uppercase text-text-h">
                            {title}
                        </h2>
                    </div>
                    <button
                        type="button"
                        onClick={onClose}
                        aria-label="Close"
                        className="rounded-lg p-1 text-text transition hover:bg-hover hover:text-text-h active:scale-95"
                    >
                        <X className="h-4 w-4" />
                    </button>
                </div>
                {children}
            </div>
        </div>
    );
};

const fieldClass =
    'w-full rounded-xl border border-border bg-bg px-3 py-2 text-sm text-text-h outline-none transition focus:border-accent-border focus:ring-1 focus:ring-accent-border';
const labelClass = 'mb-1.5 block text-[10px] font-mono uppercase tracking-wider text-text';

interface SprintFormModalProps {
    sprint: Sprint | null;
    saving: boolean;
    onClose: () => void;
    onSubmit: (input: SprintInput) => void;
}

const SprintFormModal: React.FC<SprintFormModalProps> = ({ sprint, saving, onClose, onSubmit }) => {
    const [name, setName] = useState(sprint?.name ?? '');
    const [goal, setGoal] = useState(sprint?.goal ?? '');
    const [startsOn, setStartsOn] = useState(toDateInput(sprint?.starts_on));
    const [endsOn, setEndsOn] = useState(toDateInput(sprint?.ends_on));
    const [capacity, setCapacity] = useState(
        sprint?.capacity_hours !== undefined && sprint.capacity_hours !== null
            ? String(sprint.capacity_hours)
            : ''
    );
    const [status, setStatus] = useState<SprintStatus>(sprint?.status ?? 'PLANNED');
    const [invalid, setInvalid] = useState<string | null>(null);

    const submit = (e: React.FormEvent<HTMLFormElement>) => {
        e.preventDefault();
        if (!name.trim()) {
            setInvalid('Give the sprint a name.');
            return;
        }
        if (!startsOn || !endsOn) {
            setInvalid('Both a start and an end date are required.');
            return;
        }
        if (endsOn < startsOn) {
            setInvalid('The end date cannot be before the start date.');
            return;
        }
        const parsedCapacity = capacity.trim() === '' ? null : Number(capacity);
        if (parsedCapacity !== null && (Number.isNaN(parsedCapacity) || parsedCapacity < 0)) {
            setInvalid('Capacity must be a positive number of hours.');
            return;
        }
        setInvalid(null);
        onSubmit({
            name: name.trim(),
            goal: goal.trim() === '' ? null : goal.trim(),
            starts_on: startsOn,
            ends_on: endsOn,
            capacity_hours: parsedCapacity,
            status,
        });
    };

    return (
        <ModalShell
            icon={Target}
            title={sprint ? 'Edit sprint' : 'New sprint'}
            onClose={saving ? () => undefined : onClose}
        >
            <form onSubmit={submit} noValidate className="mt-4 space-y-4">
                <div>
                    <label className={labelClass} htmlFor="sprint-name">
                        Name
                    </label>
                    <input
                        id="sprint-name"
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        placeholder="Sprint 12 — checkout polish"
                        className={fieldClass}
                        autoFocus
                    />
                </div>

                <div>
                    <label className={labelClass} htmlFor="sprint-goal">
                        Goal
                    </label>
                    <textarea
                        id="sprint-goal"
                        value={goal}
                        onChange={(e) => setGoal(e.target.value)}
                        rows={3}
                        placeholder="What does done look like at the end of this sprint?"
                        className={`${fieldClass} resize-none`}
                    />
                </div>

                <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                    <div>
                        <label className={labelClass} htmlFor="sprint-start">
                            Starts on
                        </label>
                        <input
                            id="sprint-start"
                            type="date"
                            value={startsOn}
                            onChange={(e) => setStartsOn(e.target.value)}
                            className={fieldClass}
                        />
                    </div>
                    <div>
                        <label className={labelClass} htmlFor="sprint-end">
                            Ends on
                        </label>
                        <input
                            id="sprint-end"
                            type="date"
                            value={endsOn}
                            min={startsOn || undefined}
                            onChange={(e) => setEndsOn(e.target.value)}
                            className={fieldClass}
                        />
                    </div>
                    <div>
                        <label className={labelClass} htmlFor="sprint-capacity">
                            Capacity (hours)
                        </label>
                        <input
                            id="sprint-capacity"
                            type="number"
                            min={0}
                            step="0.5"
                            value={capacity}
                            onChange={(e) => setCapacity(e.target.value)}
                            placeholder="40"
                            className={fieldClass}
                        />
                    </div>
                    <div>
                        <label className={labelClass} htmlFor="sprint-status">
                            Status
                        </label>
                        <select
                            id="sprint-status"
                            value={status}
                            onChange={(e) => setStatus(e.target.value as SprintStatus)}
                            className={fieldClass}
                        >
                            {SPRINT_STATUSES.map((s) => (
                                <option key={s} value={s}>
                                    {s}
                                </option>
                            ))}
                        </select>
                    </div>
                </div>

                {invalid && (
                    <p className="flex items-start gap-2 rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                        <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                        <span>{invalid}</span>
                    </p>
                )}

                <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
                    <button
                        type="button"
                        onClick={onClose}
                        disabled={saving}
                        className="rounded-xl px-3 py-2 text-xs font-semibold text-text transition hover:text-text-h disabled:opacity-50"
                    >
                        Cancel
                    </button>
                    <button
                        type="submit"
                        disabled={saving}
                        className="flex items-center gap-1.5 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                    >
                        {saving ? (
                            <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                        ) : (
                            <Plus className="h-3.5 w-3.5" strokeWidth={3} />
                        )}
                        <span>{sprint ? 'Save changes' : 'Create sprint'}</span>
                    </button>
                </div>
            </form>
        </ModalShell>
    );
};

const ConfirmDeleteModal: React.FC<{
    sprint: Sprint;
    busy: boolean;
    onClose: () => void;
    onConfirm: () => void;
}> = ({ sprint, busy, onClose, onConfirm }) => (
    <ModalShell icon={Trash2} title="Delete sprint" onClose={busy ? () => undefined : onClose}>
        <div className="mt-4 space-y-4">
            <p className="text-xs leading-relaxed text-text">
                Delete <span className="font-semibold text-text-h">{sprint.name}</span>? This cannot
                be undone.
            </p>
            <p className="rounded-xl border border-border bg-code-bg px-3 py-2 text-xs leading-relaxed text-text">
                The sprint&apos;s tasks are kept — they are simply detached from the sprint and stay
                on your board.
            </p>
            <div className="flex items-center justify-end gap-2 border-t border-border pt-4">
                <button
                    type="button"
                    onClick={onClose}
                    disabled={busy}
                    className="rounded-xl px-3 py-2 text-xs font-semibold text-text transition hover:text-text-h disabled:opacity-50"
                >
                    Cancel
                </button>
                <button
                    type="button"
                    onClick={onConfirm}
                    disabled={busy}
                    className="flex items-center gap-1.5 rounded-xl bg-danger px-4 py-2 text-xs font-bold text-bg transition hover:bg-danger/90 active:scale-95 disabled:opacity-50"
                >
                    {busy ? (
                        <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                    ) : (
                        <Trash2 className="h-3.5 w-3.5" />
                    )}
                    <span>Delete sprint</span>
                </button>
            </div>
        </div>
    </ModalShell>
);

/* ------------------------------- task listing -------------------------------- */

const TaskRow: React.FC<{ task: Task; onOpenTask?: (taskId: string) => void }> = ({
    task,
    onOpenTask,
}) => (
    <button
        type="button"
        disabled={!onOpenTask}
        onClick={() => onOpenTask?.(task.id)}
        className="w-full rounded-xl border border-border bg-bg px-3 py-2 text-left transition enabled:hover:border-text-h/20 enabled:hover:bg-hover enabled:active:scale-[0.99] disabled:cursor-default"
    >
        <span className="block text-xs font-semibold leading-snug text-text-h">{task.title}</span>
        <span className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[10px] text-text">
            <span className="inline-flex items-center gap-1.5">
                <span
                    aria-hidden="true"
                    className="h-2 w-2 shrink-0 rounded-full"
                    style={{ background: PRIORITY_TOKEN[task.priority] }}
                />
                <span className="font-mono uppercase tracking-wider">
                    {PRIORITY_LABELS[task.priority] ?? task.priority}
                </span>
            </span>
            <span className="inline-flex items-center gap-1">
                <Timer className="h-3 w-3 shrink-0 text-text/70" />
                <span className="tabular-nums">{formatHours(task.estimate_hours)}</span>
            </span>
            {task.reviewer && (
                <span className="inline-flex min-w-0 items-center gap-1">
                    <User className="h-3 w-3 shrink-0 text-text/70" />
                    <span className="truncate">{task.reviewer}</span>
                </span>
            )}
        </span>
    </button>
);

/* --------------------------------- the view ---------------------------------- */

export const SprintsView: React.FC<SprintsViewProps> = ({ onOpenTask }) => {
    const { showToast } = useToast();

    const [sprints, setSprints] = useState<Sprint[]>([]);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);

    const [expandedId, setExpandedId] = useState<string | null>(null);
    const [details, setDetails] = useState<Record<string, Sprint>>({});
    const [detailLoadingId, setDetailLoadingId] = useState<string | null>(null);
    const [detailErrors, setDetailErrors] = useState<Record<string, string>>({});
    const [openRationale, setOpenRationale] = useState<Record<string, boolean>>({});

    const [formFor, setFormFor] = useState<{ sprint: Sprint | null } | null>(null);
    const [saving, setSaving] = useState(false);
    const [deleting, setDeleting] = useState<Sprint | null>(null);
    const [deleteBusy, setDeleteBusy] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        setLoadError(null);
        try {
            const data = await listSprints();
            setSprints(Array.isArray(data) ? data : []);
        } catch (err) {
            setSprints([]);
            setLoadError(errorMessage(err, 'Failed to load sprints'));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void load();
    }, [load]);

    const loadDetail = useCallback(async (id: string) => {
        setDetailLoadingId(id);
        setDetailErrors((prev) => {
            const next = { ...prev };
            delete next[id];
            return next;
        });
        try {
            const full = await getSprint(id);
            setDetails((prev) => ({ ...prev, [id]: full }));
        } catch (err) {
            setDetailErrors((prev) => ({
                ...prev,
                [id]: errorMessage(err, 'Failed to load this sprint'),
            }));
        } finally {
            setDetailLoadingId((current) => (current === id ? null : current));
        }
    }, []);

    const toggleExpand = (sprint: Sprint) => {
        if (expandedId === sprint.id) {
            setExpandedId(null);
            return;
        }
        setExpandedId(sprint.id);
        if (!details[sprint.id]) void loadDetail(sprint.id);
    };

    const ordered = useMemo(
        () =>
            [...sprints].sort((a, b) => {
                const rank = (STATUS_RANK[a.status] ?? 9) - (STATUS_RANK[b.status] ?? 9);
                if (rank !== 0) return rank;
                return toDateInput(b.starts_on).localeCompare(toDateInput(a.starts_on));
            }),
        [sprints]
    );

    const handleSubmit = async (input: SprintInput) => {
        const editing = formFor?.sprint ?? null;
        setSaving(true);
        try {
            if (editing) {
                await updateSprint(editing.id, input);
                showToast('Sprint updated', 'success');
                if (details[editing.id]) void loadDetail(editing.id);
            } else {
                await createSprint(input);
                showToast('Sprint created', 'success');
            }
            setFormFor(null);
            await load();
        } catch (err) {
            showToast(errorMessage(err, 'Failed to save the sprint'), 'error');
        } finally {
            setSaving(false);
        }
    };

    const handleDelete = async () => {
        if (!deleting) return;
        const { id } = deleting;
        setDeleteBusy(true);
        try {
            await deleteSprint(id);
            showToast('Sprint deleted — its tasks were kept', 'info');
            setDeleting(null);
            setExpandedId((current) => (current === id ? null : current));
            setDetails((prev) => {
                const next = { ...prev };
                delete next[id];
                return next;
            });
            await load();
        } catch (err) {
            showToast(errorMessage(err, 'Failed to delete the sprint'), 'error');
        } finally {
            setDeleteBusy(false);
        }
    };

    const renderStats = (stats: SprintStats | undefined, capacity?: number) => {
        if (!stats) {
            return (
                <p className="mt-4 rounded-xl border border-border bg-code-bg px-3 py-2 text-[10px] font-mono uppercase tracking-wider text-text">
                    No stats reported for this sprint yet
                </p>
            );
        }

        const total = stats.total_tasks ?? 0;
        const done = stats.done_tasks ?? 0;
        const completion = total > 0 ? done / total : 0;
        const load = stats.load_percent ?? 0;
        const over = load > 100;
        // Stretch the track past 100 when overloaded so the capacity mark stays visible.
        const scale = Math.max(100, load);

        return (
            <>
                <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
                    <Stat
                        icon={ListChecks}
                        label="Tasks"
                        value={String(total)}
                        hint={`${done} done`}
                    />
                    <Stat icon={CircleCheckBig} label="Done" value={String(done)} />
                    <Stat icon={Timer} label="Planned" value={formatHours(stats.planned_hours)} />
                    <Stat icon={Layers} label="Buffer" value={formatHours(stats.buffer_hours)} />
                    <Stat
                        icon={Gauge}
                        label="Committed"
                        value={formatHours(stats.committed_hours)}
                        hint={capacity ? `of ${formatHours(capacity)}` : undefined}
                    />
                </div>

                <div className="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-2">
                    <Meter
                        label="Completion"
                        valueLabel={`${done}/${total} · ${Math.round(completion * 100)}%`}
                        ratio={completion}
                        fill="var(--viz-seq-500)"
                        track="var(--viz-seq-100)"
                    />
                    <Meter
                        label="Load vs capacity"
                        valueLabel={`${Math.round(load)}%`}
                        ratio={load / scale}
                        tick={100 / scale}
                        fill={over ? 'var(--viz-warning)' : 'var(--viz-seq-500)'}
                        track={
                            over
                                ? 'color-mix(in oklab, var(--viz-warning) 20%, transparent)'
                                : 'var(--viz-seq-100)'
                        }
                        note={
                            over
                                ? 'Over capacity — trim scope or extend the sprint'
                                : capacity
                                  ? `Capacity ${formatHours(capacity)}`
                                  : 'No capacity set'
                        }
                    />
                </div>
            </>
        );
    };

    const renderExpanded = (sprint: Sprint) => {
        const detail = details[sprint.id];
        const detailError = detailErrors[sprint.id];

        if (detailLoadingId === sprint.id && !detail) {
            return (
                <div className="flex items-center gap-2 px-4 py-6 text-xs text-text sm:px-5">
                    <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
                    <span>Loading sprint tasks…</span>
                </div>
            );
        }

        if (detailError) {
            return (
                <div className="px-4 py-5 sm:px-5">
                    <p className="flex flex-wrap items-center gap-2 rounded-xl border border-danger/30 bg-danger/10 px-3 py-2 text-xs text-danger">
                        <TriangleAlert className="h-3.5 w-3.5 shrink-0" />
                        <span>{detailError}</span>
                        <button
                            type="button"
                            onClick={() => void loadDetail(sprint.id)}
                            className="ml-auto rounded-lg border border-danger/30 px-2 py-1 text-[10px] font-mono uppercase tracking-wider transition hover:bg-danger/10 active:scale-95"
                        >
                            Retry
                        </button>
                    </p>
                </div>
            );
        }

        if (!detail) return null;

        const tasks = detail.tasks ?? [];
        const groups = BOARD_STATUSES.map((status) => ({
            status,
            items: tasks.filter((t) => t.status === status),
        })).filter((group) => group.items.length > 0);

        if (groups.length === 0) {
            return (
                <div className="px-4 py-5 sm:px-5">
                    <p className="rounded-xl border border-border bg-code-bg px-3 py-3 text-xs leading-relaxed text-text">
                        No tasks are attached to this sprint yet. Commit a plan from the AI Planner,
                        or set the sprint on a task from the board.
                    </p>
                </div>
            );
        }

        return (
            <div className="grid grid-cols-1 gap-4 px-4 py-5 sm:grid-cols-2 sm:px-5 xl:grid-cols-4">
                {groups.map((group) => (
                    <section key={group.status} className="min-w-0">
                        <h4 className="flex items-center justify-between gap-2 border-b border-border pb-2 text-[10px] font-mono uppercase tracking-wider text-text">
                            <span>{STATUS_LABELS[group.status]}</span>
                            <span className="tabular-nums text-text/70">{group.items.length}</span>
                        </h4>
                        <div className="mt-2 space-y-2">
                            {group.items.map((task) => (
                                <TaskRow key={task.id} task={task} onOpenTask={onOpenTask} />
                            ))}
                        </div>
                    </section>
                ))}
            </div>
        );
    };

    const renderCard = (sprint: Sprint) => {
        const expanded = expandedId === sprint.id;
        const detail = details[sprint.id];
        const rationale = detail?.ai_rationale ?? sprint.ai_rationale;
        const days = spanDays(sprint.starts_on, sprint.ends_on);
        const rationaleOpen = !!openRationale[sprint.id];

        return (
            <article
                key={sprint.id}
                className="overflow-hidden rounded-2xl border border-border bg-surface shadow-(--shadow) transition-colors"
            >
                <div className="p-4 sm:p-5">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                        <button
                            type="button"
                            onClick={() => toggleExpand(sprint)}
                            aria-expanded={expanded}
                            className="group flex min-w-0 flex-1 items-start gap-2 text-left"
                        >
                            <ChevronDown
                                className={`mt-0.5 h-4 w-4 shrink-0 text-text transition-transform duration-300 ${
                                    expanded ? 'rotate-0' : '-rotate-90'
                                }`}
                            />
                            <span className="min-w-0">
                                <span className="block break-words text-sm font-semibold leading-snug text-text-h transition group-hover:text-accent sm:text-base">
                                    {sprint.name}
                                </span>
                                <span className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-[10px] font-mono uppercase tracking-wider text-text">
                                    <CalendarRange className="h-3.5 w-3.5 shrink-0 text-text/70" />
                                    <span>
                                        {formatDay(sprint.starts_on)} – {formatDay(sprint.ends_on, true)}
                                    </span>
                                    {days !== null && (
                                        <span className="text-text/70">· {days}d</span>
                                    )}
                                </span>
                            </span>
                        </button>

                        <div className="flex shrink-0 items-center gap-2">
                            <StatusChip status={sprint.status} />
                            <button
                                type="button"
                                onClick={() => setFormFor({ sprint })}
                                aria-label={`Edit ${sprint.name}`}
                                title="Edit sprint"
                                className="rounded-xl border border-border bg-hover p-2 text-text transition hover:text-text-h active:scale-95"
                            >
                                <Pencil className="h-3.5 w-3.5" />
                            </button>
                            <button
                                type="button"
                                onClick={() => setDeleting(sprint)}
                                aria-label={`Delete ${sprint.name}`}
                                title="Delete sprint"
                                className="rounded-xl border border-border bg-hover p-2 text-text transition hover:border-danger/40 hover:bg-danger/10 hover:text-danger active:scale-95"
                            >
                                <Trash2 className="h-3.5 w-3.5" />
                            </button>
                        </div>
                    </div>

                    {sprint.goal && (
                        <p className="mt-3 text-xs leading-relaxed text-text">{sprint.goal}</p>
                    )}

                    {renderStats(sprint.stats ?? detail?.stats, sprint.capacity_hours)}

                    {rationale && (
                        <div className="mt-4 rounded-xl border border-border bg-code-bg">
                            <button
                                type="button"
                                onClick={() =>
                                    setOpenRationale((prev) => ({
                                        ...prev,
                                        [sprint.id]: !prev[sprint.id],
                                    }))
                                }
                                aria-expanded={rationaleOpen}
                                className="flex w-full items-center gap-2 px-3 py-2 text-left"
                            >
                                <Sparkles className="h-3.5 w-3.5 shrink-0 text-accent" />
                                <span className="min-w-0 flex-1 truncate text-[10px] font-mono uppercase tracking-wider text-text">
                                    Why the AI planned it this way
                                </span>
                                <ChevronDown
                                    className={`h-3.5 w-3.5 shrink-0 text-text transition-transform duration-300 ${
                                        rationaleOpen ? 'rotate-0' : '-rotate-90'
                                    }`}
                                />
                            </button>
                            {rationaleOpen && (
                                <p className="whitespace-pre-wrap border-t border-border px-3 py-3 text-xs leading-relaxed text-text">
                                    {rationale}
                                </p>
                            )}
                        </div>
                    )}
                </div>

                {expanded && (
                    <div className="border-t border-border bg-bg/40">{renderExpanded(sprint)}</div>
                )}
            </article>
        );
    };

    return (
        <div className="space-y-5">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2.5">
                    <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-hover text-success shadow-inner">
                        <Target className="h-4 w-4" />
                    </div>
                    <div className="min-w-0">
                        <h2 className="truncate text-base font-bold tracking-tight text-text-h sm:text-lg">
                            Sprints
                        </h2>
                        <p className="text-[10px] font-mono uppercase tracking-wider text-text">
                            {loading ? 'Loading…' : `${sprints.length} total`}
                        </p>
                    </div>
                </div>

                <div className="flex shrink-0 items-center gap-2">
                    <button
                        type="button"
                        onClick={() => void load()}
                        disabled={loading}
                        title="Reload sprints"
                        aria-label="Reload sprints"
                        className="rounded-xl border border-border bg-hover p-2 text-text transition hover:text-text-h active:scale-95 disabled:opacity-50"
                    >
                        <RefreshCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
                    </button>
                    <button
                        type="button"
                        onClick={() => setFormFor({ sprint: null })}
                        className="flex items-center gap-1.5 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-lg shadow-success/10 transition hover:bg-success/90 active:scale-95"
                    >
                        <Plus className="h-3.5 w-3.5" strokeWidth={3} />
                        <span>New sprint</span>
                    </button>
                </div>
            </div>

            {loading ? (
                <div className="space-y-4">
                    {[0, 1, 2].map((i) => (
                        <div
                            key={i}
                            className="animate-pulse rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) sm:p-5"
                        >
                            <div className="h-4 w-1/3 rounded-lg bg-hover" />
                            <div className="mt-3 h-3 w-2/3 rounded-lg bg-hover" />
                            <div className="mt-4 grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5">
                                {[0, 1, 2, 3, 4].map((c) => (
                                    <div key={c} className="h-14 rounded-xl bg-hover" />
                                ))}
                            </div>
                        </div>
                    ))}
                </div>
            ) : loadError ? (
                <div className="flex flex-col items-center rounded-2xl border border-border bg-surface p-8 text-center shadow-(--shadow)">
                    <div className="rounded-2xl border border-danger/30 bg-danger/10 p-3 text-danger">
                        <TriangleAlert className="h-5 w-5" />
                    </div>
                    <h3 className="mt-4 text-sm font-bold text-text-h">Could not load sprints</h3>
                    <p className="mt-1 max-w-md text-xs leading-relaxed text-text">{loadError}</p>
                    <button
                        type="button"
                        onClick={() => void load()}
                        className="mt-5 flex items-center gap-1.5 rounded-xl border border-border bg-hover px-4 py-2 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                    >
                        <RefreshCw className="h-3.5 w-3.5" />
                        <span>Try again</span>
                    </button>
                </div>
            ) : ordered.length === 0 ? (
                <div className="flex flex-col items-center rounded-2xl border border-border bg-surface p-8 text-center shadow-(--shadow) sm:p-12">
                    <div className="rounded-2xl border border-border bg-code-bg p-4 text-accent shadow-xl">
                        <Sparkles className="h-6 w-6" />
                    </div>
                    <h3 className="mt-4 text-base font-bold tracking-tight text-text-h">
                        No sprints yet
                    </h3>
                    <p className="mt-2 max-w-md text-xs leading-relaxed text-text sm:text-sm">
                        Open <span className="font-semibold text-text-h">AI Planner</span> in the top
                        nav: it interviews you about blockers, estimates and the buffer you want, then
                        commits a balanced sprint straight to this page. You can always build one by
                        hand instead.
                    </p>
                    <button
                        type="button"
                        onClick={() => setFormFor({ sprint: null })}
                        className="mt-6 flex items-center gap-1.5 rounded-xl border border-border bg-hover px-4 py-2 text-xs font-semibold text-text-h transition hover:border-text-h/20 active:scale-95"
                    >
                        <Plus className="h-3.5 w-3.5" strokeWidth={3} />
                        <span>Create a sprint manually</span>
                    </button>
                </div>
            ) : (
                <div className="space-y-4">{ordered.map(renderCard)}</div>
            )}

            {formFor && (
                <SprintFormModal
                    key={formFor.sprint?.id ?? 'new'}
                    sprint={formFor.sprint}
                    saving={saving}
                    onClose={() => setFormFor(null)}
                    onSubmit={(input) => void handleSubmit(input)}
                />
            )}

            {deleting && (
                <ConfirmDeleteModal
                    sprint={deleting}
                    busy={deleteBusy}
                    onClose={() => setDeleting(null)}
                    onConfirm={() => void handleDelete()}
                />
            )}
        </div>
    );
};

import React from 'react';
import {
    AlertTriangle,
    CalendarDays,
    GripVertical,
    Shield,
    Timer,
    Trash2,
} from 'lucide-react';
import type { Task, TaskPriority, TaskStatus } from '../types';
import { BOARD_STATUSES, PRIORITY_LABELS, STATUS_LABELS } from '../types';

/* ------------------------------ shared tokens ------------------------------ */

/** Column / badge accent per status. Reused by KanbanColumn. */
export const STATUS_DOT: Record<TaskStatus, string> = {
    TODO: 'bg-text/40',
    IN_PROGRESS: 'bg-warning',
    IN_REVIEW: 'bg-info',
    DONE: 'bg-success',
};

export const STATUS_BADGE: Record<TaskStatus, string> = {
    TODO: 'bg-hover text-text border-border',
    IN_PROGRESS: 'bg-warning/10 text-warning border-warning/30',
    IN_REVIEW: 'bg-info/10 text-info border-info/30',
    DONE: 'bg-success/10 text-success border-success/30',
};

/**
 * URGENT / HIGH borrow the reserved status hues; MEDIUM / LOW stay muted.
 * The hue is mixed toward the page ink so it stays legible in both themes —
 * the priority word is always rendered, colour is never the only signal.
 */
const PRIORITY_TONE: Partial<Record<TaskPriority, React.CSSProperties>> = {
    URGENT: {
        color: 'color-mix(in srgb, var(--viz-critical) 88%, var(--text-h))',
        backgroundColor: 'color-mix(in srgb, var(--viz-critical) 14%, transparent)',
        borderColor: 'color-mix(in srgb, var(--viz-critical) 45%, transparent)',
    },
    HIGH: {
        color: 'color-mix(in srgb, var(--viz-serious) 72%, var(--text-h))',
        backgroundColor: 'color-mix(in srgb, var(--viz-serious) 16%, transparent)',
        borderColor: 'color-mix(in srgb, var(--viz-serious) 48%, transparent)',
    },
};

const OVERDUE_TONE: React.CSSProperties = {
    color: 'color-mix(in srgb, var(--viz-critical) 88%, var(--text-h))',
    backgroundColor: 'color-mix(in srgb, var(--viz-critical) 12%, transparent)',
    borderColor: 'color-mix(in srgb, var(--viz-critical) 42%, transparent)',
};

/* -------------------------------- helpers ---------------------------------- */

export const parseDate = (value?: string | null): Date | null => {
    if (!value) return null;
    const d = new Date(value);
    return Number.isNaN(d.getTime()) ? null : d;
};

export const formatHours = (h: number): string =>
    `${Number.isInteger(h) ? h : Math.round(h * 10) / 10}h`;

export const initials = (name: string): string => {
    const parts = name.trim().split(/\s+/).filter(Boolean);
    if (parts.length === 0) return '?';
    if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
    return `${parts[0][0]}${parts[parts.length - 1][0]}`.toUpperCase();
};

const startOfToday = (): number => {
    const d = new Date();
    d.setHours(0, 0, 0, 0);
    return d.getTime();
};

const formatDay = (d: Date): string =>
    d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });

/* ---------------------------------- chip ----------------------------------- */

interface ChipProps {
    children: React.ReactNode;
    title?: string;
    className?: string;
    style?: React.CSSProperties;
}

const Chip: React.FC<ChipProps> = ({ children, title, className, style }) => (
    <span
        title={title}
        style={style}
        className={`inline-flex max-w-full items-center gap-1 rounded-lg border px-1.5 py-0.5 text-[10px] font-mono font-medium tracking-wide ${
            className ?? 'border-border bg-hover text-text'
        }`}
    >
        {children}
    </span>
);

/* --------------------------------- card ------------------------------------ */

export interface TaskCardProps {
    task: Task;
    onStatusChange: (task: Task, status: TaskStatus) => void;
    onOpen: (task: Task) => void;
    onDelete?: (id: string) => void;
    /** Drag wiring supplied by KanbanColumn; omitted the card is simply static. */
    draggable?: boolean;
    isDragging?: boolean;
    onDragStart?: (e: React.DragEvent<HTMLDivElement>) => void;
    onDragEnd?: (e: React.DragEvent<HTMLDivElement>) => void;
    onDragOver?: (e: React.DragEvent<HTMLDivElement>) => void;
    onDrop?: (e: React.DragEvent<HTMLDivElement>) => void;
}

export const TaskCard: React.FC<TaskCardProps> = ({
    task,
    onStatusChange,
    onOpen,
    onDelete,
    draggable = false,
    isDragging = false,
    onDragStart,
    onDragEnd,
    onDragOver,
    onDrop,
}) => {
    const due = parseDate(task.due_date);
    const overdue = due !== null && task.status !== 'DONE' && due.getTime() < startOfToday();
    const reviewer = task.reviewer?.trim();
    const blockers = task.blockers?.trim();
    const priority: TaskPriority = task.priority ?? 'MEDIUM';
    const priorityTone = PRIORITY_TONE[priority];

    const stop = (e: React.SyntheticEvent) => e.stopPropagation();

    const handleKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            onOpen(task);
        }
    };

    const handleDragStart = (e: React.DragEvent<HTMLDivElement>) => {
        // Never start a drag from the status select or the delete button.
        const target = e.target as HTMLElement | null;
        if (target?.closest('[data-no-drag="true"]')) {
            e.preventDefault();
            return;
        }
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', task.id);
        onDragStart?.(e);
    };

    return (
        <div
            data-card-id={task.id}
            draggable={draggable}
            onDragStart={draggable ? handleDragStart : undefined}
            onDragEnd={onDragEnd}
            onDragOver={onDragOver}
            onDrop={onDrop}
            tabIndex={0}
            aria-label={`${task.title} — ${STATUS_LABELS[task.status]}, ${PRIORITY_LABELS[priority]} priority`}
            onClick={() => onOpen(task)}
            onKeyDown={handleKeyDown}
            className={`group relative flex cursor-pointer flex-col rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) transition-colors hover:bg-hover focus:outline-none focus-visible:ring-2 focus-visible:ring-accent ${
                isDragging ? 'opacity-40' : ''
            }`}
        >
            <div className="flex items-start justify-between gap-2">
                <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                    <span
                        className={`inline-flex items-center rounded-lg border px-2 py-0.5 text-[10px] font-mono font-medium uppercase tracking-wider ${STATUS_BADGE[task.status]}`}
                    >
                        {STATUS_LABELS[task.status]}
                    </span>
                    <span
                        style={priorityTone}
                        className={`inline-flex items-center rounded-lg border px-2 py-0.5 text-[10px] font-mono font-semibold uppercase tracking-wider ${
                            priorityTone ? '' : 'border-border bg-hover text-text'
                        }`}
                    >
                        {PRIORITY_LABELS[priority]}
                    </span>
                </div>

                {draggable && (
                    <GripVertical
                        aria-hidden="true"
                        className="h-4 w-4 shrink-0 cursor-grab text-text/40 opacity-0 transition group-hover:opacity-100 active:cursor-grabbing"
                    />
                )}
            </div>

            <h3
                className={`mt-3 text-sm font-semibold leading-snug break-words ${
                    task.status === 'DONE' ? 'text-text/60 line-through' : 'text-text-h'
                }`}
            >
                {task.title}
            </h3>

            {task.description && (
                <p className="mt-1.5 line-clamp-2 text-xs leading-relaxed text-text break-words">
                    {task.description}
                </p>
            )}

            {(task.estimate_hours != null ||
                task.buffer_hours != null ||
                due !== null ||
                reviewer ||
                blockers) && (
                <div className="mt-3 flex flex-wrap items-center gap-1.5">
                    {task.estimate_hours != null && (
                        <Chip title={`Estimate ${formatHours(task.estimate_hours)}`}>
                            <Timer aria-hidden="true" className="h-3 w-3" />
                            {formatHours(task.estimate_hours)}
                        </Chip>
                    )}

                    {task.buffer_hours != null && (
                        <Chip title={`Buffer ${formatHours(task.buffer_hours)}`}>
                            <Shield aria-hidden="true" className="h-3 w-3" />
                            {`+${formatHours(task.buffer_hours)}`}
                        </Chip>
                    )}

                    {due !== null && (
                        <Chip
                            title={overdue ? `Overdue — due ${formatDay(due)}` : `Due ${formatDay(due)}`}
                            style={overdue ? OVERDUE_TONE : undefined}
                            className={overdue ? '' : undefined}
                        >
                            <CalendarDays aria-hidden="true" className="h-3 w-3" />
                            {overdue ? `Overdue ${formatDay(due)}` : formatDay(due)}
                        </Chip>
                    )}

                    {reviewer && (
                        <Chip title={`Reviewer: ${reviewer}`}>
                            <span
                                aria-hidden="true"
                                className="flex h-4 w-4 shrink-0 items-center justify-center rounded-full bg-info/15 text-[8px] font-bold text-info"
                            >
                                {initials(reviewer)}
                            </span>
                            <span className="truncate max-w-[8rem]">{reviewer}</span>
                        </Chip>
                    )}

                    {blockers && (
                        <Chip
                            title={`Blocked: ${blockers}`}
                            className="border-danger/30 bg-danger/10 text-danger"
                        >
                            <AlertTriangle aria-hidden="true" className="h-3 w-3" />
                            Blocked
                        </Chip>
                    )}
                </div>
            )}

            {/* Touch + keyboard fallback for drag and drop — always present. */}
            <div
                data-no-drag="true"
                draggable={false}
                onClick={stop}
                onKeyDown={stop}
                className="mt-3 flex items-center justify-between gap-2 border-t border-border/60 pt-3"
            >
                <label htmlFor={`status-${task.id}`} className="sr-only">
                    Status for {task.title}
                </label>
                <select
                    id={`status-${task.id}`}
                    value={task.status}
                    onChange={(e) => onStatusChange(task, e.target.value as TaskStatus)}
                    className="min-w-0 flex-1 rounded-lg border border-border bg-bg px-2 py-1 text-xs font-mono text-text-h outline-none transition focus:border-accent"
                >
                    {BOARD_STATUSES.map((s) => (
                        <option key={s} value={s}>
                            {STATUS_LABELS[s]}
                        </option>
                    ))}
                </select>

                {onDelete && (
                    <button
                        type="button"
                        onClick={() => onDelete(task.id)}
                        title="Delete task"
                        aria-label={`Delete ${task.title}`}
                        className="shrink-0 rounded-lg border border-border p-1.5 text-text transition hover:border-danger/40 hover:bg-danger/10 hover:text-danger active:scale-95"
                    >
                        <Trash2 className="h-3.5 w-3.5" />
                    </button>
                )}
            </div>
        </div>
    );
};

import React, { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Plus, SquareKanban, UserCheck } from 'lucide-react';
import type { Sprint, Task, TaskInput, TaskStatus } from '../../types';
import { BOARD_STATUSES, STATUS_LABELS } from '../../types';
import { useToast } from '../../context/ToastContext';
import { KanbanColumn } from './KanbanColumn';
import { TaskModal } from './TaskModal';

export interface KanbanBoardProps {
    tasks: Task[];
    onMove: (
        id: string,
        status: TaskStatus,
        afterId: string | null,
        beforeId: string | null
    ) => Promise<void>;
    onUpdate: (id: string, patch: Partial<TaskInput>) => Promise<void>;
    onDelete: (id: string) => Promise<void>;
    onCreate: (input: TaskInput) => Promise<void>;
    sprints?: Sprint[];
    loading?: boolean;
}

interface PendingMove {
    task: Task;
    status: TaskStatus;
    insertAt: number;
    afterId: string | null;
    beforeId: string | null;
}

interface ModalState {
    task: Task | null;
    defaultStatus?: TaskStatus;
}

type Columns = Record<TaskStatus, Task[]>;

const byPosition = (a: Task, b: Task): number => {
    const delta = (a.position ?? 0) - (b.position ?? 0);
    if (delta !== 0) return delta;
    return (a.created_at ?? '').localeCompare(b.created_at ?? '');
};

export const errorMessage = (err: unknown, fallback: string): string => {
    if (typeof err === 'object' && err !== null) {
        const response = (err as { response?: { data?: { error?: string } } }).response;
        if (typeof response?.data?.error === 'string' && response.data.error) {
            return response.data.error;
        }
    }
    return fallback;
};

/** Pure local reorder used for the optimistic update; positions are re-indexed. */
const reorder = (
    list: Task[],
    id: string,
    status: TaskStatus,
    insertAt: number,
    reviewer?: string
): Task[] => {
    const moved = list.find((t) => t.id === id);
    if (!moved) return list;

    const untouched = list.filter((t) => t.id !== id && t.status !== status);
    const dest = list.filter((t) => t.id !== id && t.status === status).sort(byPosition);
    const at = Math.max(0, Math.min(insertAt, dest.length));
    dest.splice(at, 0, { ...moved, status, reviewer: reviewer ?? moved.reviewer });

    return [...untouched, ...dest.map((t, i) => ({ ...t, position: i }))];
};

/* ------------------------------- mini dialog -------------------------------- */

interface MiniDialogProps {
    title: string;
    icon: React.ReactNode;
    children: React.ReactNode;
    onCancel: () => void;
}

const MiniDialog: React.FC<MiniDialogProps> = ({ title, icon, children, onCancel }) => (
    <div
        onMouseDown={(e) => {
            if (e.target === e.currentTarget) onCancel();
        }}
        onKeyDown={(e) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                onCancel();
            }
        }}
        style={{ backgroundColor: 'rgba(0, 0, 0, 0.55)' }}
        className="fixed inset-0 z-50 flex items-end justify-center p-0 backdrop-blur-md sm:items-center sm:p-4"
    >
        <div
            role="dialog"
            aria-modal="true"
            aria-label={title}
            className="w-full max-w-sm rounded-t-3xl border border-border bg-surface p-5 shadow-(--shadow) sm:rounded-2xl"
        >
            <div className="flex items-center gap-2.5 border-b border-border pb-3">
                <span className="rounded-xl border border-border bg-hover p-2 text-accent">{icon}</span>
                <h2 className="text-sm font-bold uppercase tracking-wider text-text-h font-mono">
                    {title}
                </h2>
            </div>
            {children}
        </div>
    </div>
);

/* ---------------------------------- board ----------------------------------- */

export const KanbanBoard: React.FC<KanbanBoardProps> = ({
    tasks,
    onMove,
    onUpdate,
    onDelete,
    onCreate,
    sprints,
    loading = false,
}) => {
    const { showToast } = useToast();

    const safeTasks = useMemo<Task[]>(
        () => (Array.isArray(tasks) ? tasks.filter((t): t is Task => Boolean(t?.id)) : []),
        [tasks]
    );

    const [localTasks, setLocalTasks] = useState<Task[]>(safeTasks);
    const [draggingId, setDraggingId] = useState<string | null>(null);
    const [dropTarget, setDropTarget] = useState<{ status: TaskStatus; index: number } | null>(null);
    const [modal, setModal] = useState<ModalState | null>(null);
    const [pendingReview, setPendingReview] = useState<PendingMove | null>(null);
    const [reviewerDraft, setReviewerDraft] = useState('');
    const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);

    // The server list is the source of truth; optimistic edits are overwritten
    // by the next fetch, which is exactly what we want.
    useEffect(() => {
        setLocalTasks(safeTasks);
    }, [safeTasks]);

    const columns = useMemo<Columns>(() => {
        const map: Columns = { TODO: [], IN_PROGRESS: [], IN_REVIEW: [], DONE: [] };
        for (const task of localTasks) {
            const bucket = map[task.status] ? task.status : 'TODO';
            map[bucket].push(task);
        }
        for (const status of BOARD_STATUSES) map[status].sort(byPosition);
        return map;
    }, [localTasks]);

    /* ----------------------------- move pipeline ----------------------------- */

    const performMove = async (pending: PendingMove, reviewer?: string) => {
        const previous = localTasks;
        setLocalTasks(reorder(previous, pending.task.id, pending.status, pending.insertAt, reviewer));

        try {
            // The reviewer must land before /move, otherwise the server rejects it.
            if (reviewer !== undefined) await onUpdate(pending.task.id, { reviewer });
            await onMove(pending.task.id, pending.status, pending.afterId, pending.beforeId);
        } catch (err) {
            setLocalTasks(previous);
            showToast(errorMessage(err, 'Could not move that task'), 'error');
        }
    };

    const requestMove = (pending: PendingMove) => {
        const hasReviewer = Boolean(pending.task.reviewer?.trim());
        if (pending.status === 'IN_REVIEW' && !hasReviewer) {
            // Nothing is sent and nothing moves until a reviewer is named.
            setReviewerDraft('');
            setPendingReview(pending);
            return;
        }
        void performMove(pending);
    };

    /** Turns a rendered insertion index into after/before ids, minus the dragged card. */
    const resolveMove = (task: Task, status: TaskStatus, index: number): PendingMove | null => {
        const dest = columns[status] ?? [];
        const dragIndex = dest.findIndex((t) => t.id === task.id);
        const rest = dest.filter((t) => t.id !== task.id);

        let insertAt = index;
        if (dragIndex !== -1 && index > dragIndex) insertAt -= 1;
        insertAt = Math.max(0, Math.min(insertAt, rest.length));

        if (dragIndex !== -1 && insertAt === dragIndex) return null; // dropped back where it was

        return {
            task,
            status,
            insertAt,
            afterId: insertAt > 0 ? rest[insertAt - 1].id : null,
            beforeId: insertAt < rest.length ? rest[insertAt].id : null,
        };
    };

    /* -------------------------------- drag ---------------------------------- */

    const handleCardDragStart = (task: Task) => setDraggingId(task.id);

    const handleCardDragEnd = () => {
        setDraggingId(null);
        setDropTarget(null);
    };

    const handleDragOverColumn = (status: TaskStatus, index: number) =>
        setDropTarget((prev) =>
            prev && prev.status === status && prev.index === index ? prev : { status, index }
        );

    const handleDragLeaveColumn = (status: TaskStatus) =>
        setDropTarget((prev) => (prev && prev.status === status ? null : prev));

    const handleDropColumn = (status: TaskStatus, index: number) => {
        const id = draggingId;
        setDraggingId(null);
        setDropTarget(null);
        if (!id) return;

        const task = localTasks.find((t) => t.id === id);
        if (!task) return;

        const pending = resolveMove(task, status, index);
        if (pending) requestMove(pending);
    };

    /* ------------------------- select / keyboard path ------------------------ */

    const handleStatusChange = (task: Task, status: TaskStatus) => {
        if (status === task.status) return;
        const rest = (columns[status] ?? []).filter((t) => t.id !== task.id);
        requestMove({
            task,
            status,
            insertAt: rest.length,
            afterId: rest.length > 0 ? rest[rest.length - 1].id : null,
            beforeId: null,
        });
    };

    /* ------------------------------ reviewer gate ---------------------------- */

    const confirmReviewer = async () => {
        const name = reviewerDraft.trim();
        if (!name || !pendingReview) return;
        const pending = pendingReview;
        setPendingReview(null);
        setReviewerDraft('');
        await performMove(pending, name);
    };

    /* --------------------------------- crud ---------------------------------- */

    const handleModalSave = async (input: TaskInput) => {
        const editing = modal?.task;
        try {
            if (editing) await onUpdate(editing.id, input);
            else await onCreate(input);
            setModal(null);
            showToast(editing ? 'Task updated' : 'Task created', 'success');
        } catch (err) {
            showToast(errorMessage(err, 'Could not save that task'), 'error');
            throw err;
        }
    };

    const handleColumnCreate = async (input: TaskInput) => {
        try {
            await onCreate(input);
            showToast('Task created', 'success');
        } catch (err) {
            showToast(errorMessage(err, 'Could not create that task'), 'error');
            throw err;
        }
    };

    const removeTask = async (id: string) => {
        const previous = localTasks;
        setBusy(true);
        setLocalTasks(previous.filter((t) => t.id !== id));
        try {
            await onDelete(id);
            showToast('Task deleted', 'success');
        } catch (err) {
            setLocalTasks(previous);
            showToast(errorMessage(err, 'Could not delete that task'), 'error');
            throw err;
        } finally {
            setBusy(false);
        }
    };

    const total = localTasks.length;

    return (
        <div className="flex w-full min-w-0 flex-col gap-4">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex min-w-0 items-center gap-2.5">
                    <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border bg-hover text-success">
                        <SquareKanban className="h-4 w-4" />
                    </span>
                    <div className="min-w-0">
                        <h2 className="truncate text-sm font-bold uppercase tracking-wider text-text-h font-mono">
                            Board
                        </h2>
                        <p className="text-[11px] text-text">
                            {loading && total === 0
                                ? 'Loading tasks...'
                                : `${total} ${total === 1 ? 'task' : 'tasks'} across 4 columns`}
                        </p>
                    </div>
                </div>

                <button
                    type="button"
                    onClick={() => setModal({ task: null, defaultStatus: 'TODO' })}
                    className="flex shrink-0 items-center gap-1.5 rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg shadow-md shadow-success/10 transition hover:bg-success/90 active:scale-95"
                >
                    <Plus aria-hidden="true" className="h-3.5 w-3.5" strokeWidth={3} />
                    <span>New task</span>
                </button>
            </div>

            {/* The board is the only horizontal scroller; the page body never is. */}
            <div className="flex w-full min-w-0 gap-3 overflow-x-auto pb-3">
                {BOARD_STATUSES.map((status) => (
                    <KanbanColumn
                        key={status}
                        status={status}
                        tasks={columns[status]}
                        loading={loading}
                        draggingId={draggingId}
                        dropIndex={dropTarget?.status === status ? dropTarget.index : null}
                        onDragOverColumn={handleDragOverColumn}
                        onDragLeaveColumn={handleDragLeaveColumn}
                        onDropColumn={handleDropColumn}
                        onCardDragStart={handleCardDragStart}
                        onCardDragEnd={handleCardDragEnd}
                        onStatusChange={handleStatusChange}
                        onOpen={(task) => setModal({ task })}
                        onDelete={(id) => setConfirmDeleteId(id)}
                        onCreate={handleColumnCreate}
                    />
                ))}
            </div>

            {pendingReview && (
                <MiniDialog
                    title="Who reviews this?"
                    icon={<UserCheck className="h-4 w-4" />}
                    onCancel={() => {
                        setPendingReview(null);
                        setReviewerDraft('');
                    }}
                >
                    <form
                        onSubmit={(e) => {
                            e.preventDefault();
                            void confirmReviewer();
                        }}
                        className="mt-4 space-y-3"
                    >
                        <p className="text-xs leading-relaxed text-text">
                            <span className="font-semibold text-text-h">{pendingReview.task.title}</span>{' '}
                            cannot enter {STATUS_LABELS.IN_REVIEW} without a reviewer.
                        </p>
                        <input
                            type="text"
                            autoFocus
                            value={reviewerDraft}
                            onChange={(e) => setReviewerDraft(e.target.value)}
                            placeholder="Reviewer name"
                            aria-label="Reviewer name"
                            required
                            className="w-full rounded-xl border border-border bg-bg px-3 py-2 text-sm text-text-h outline-none transition focus:border-accent"
                        />
                        <div className="flex items-center justify-end gap-2">
                            <button
                                type="button"
                                onClick={() => {
                                    setPendingReview(null);
                                    setReviewerDraft('');
                                }}
                                className="rounded-xl px-3 py-2 text-xs font-semibold text-text transition hover:text-text-h active:scale-95"
                            >
                                Cancel
                            </button>
                            <button
                                type="submit"
                                disabled={reviewerDraft.trim() === ''}
                                className="rounded-xl bg-success px-4 py-2 text-xs font-bold text-bg transition hover:bg-success/90 active:scale-95 disabled:opacity-50"
                            >
                                Move to review
                            </button>
                        </div>
                    </form>
                </MiniDialog>
            )}

            {confirmDeleteId && (
                <MiniDialog
                    title="Delete task"
                    icon={<AlertTriangle className="h-4 w-4" />}
                    onCancel={() => setConfirmDeleteId(null)}
                >
                    <div className="mt-4 space-y-4">
                        <p className="text-xs leading-relaxed text-text">
                            This removes{' '}
                            <span className="font-semibold text-text-h">
                                {localTasks.find((t) => t.id === confirmDeleteId)?.title ?? 'this task'}
                            </span>{' '}
                            permanently. There is no undo.
                        </p>
                        <div className="flex items-center justify-end gap-2">
                            <button
                                type="button"
                                autoFocus
                                onClick={() => setConfirmDeleteId(null)}
                                className="rounded-xl px-3 py-2 text-xs font-semibold text-text transition hover:text-text-h active:scale-95"
                            >
                                Keep it
                            </button>
                            <button
                                type="button"
                                disabled={busy}
                                onClick={() => {
                                    const id = confirmDeleteId;
                                    setConfirmDeleteId(null);
                                    void removeTask(id).catch(() => undefined);
                                }}
                                className="rounded-xl bg-danger px-4 py-2 text-xs font-bold text-bg transition hover:opacity-90 active:scale-95 disabled:opacity-50"
                            >
                                {busy ? 'Deleting...' : 'Delete'}
                            </button>
                        </div>
                    </div>
                </MiniDialog>
            )}

            <TaskModal
                open={modal !== null}
                task={modal?.task ?? null}
                defaultStatus={modal?.defaultStatus}
                sprints={sprints}
                onClose={() => setModal(null)}
                onSave={handleModalSave}
                onDelete={
                    modal?.task
                        ? async (id: string) => {
                              await removeTask(id);
                              setModal(null);
                          }
                        : undefined
                }
            />
        </div>
    );
};

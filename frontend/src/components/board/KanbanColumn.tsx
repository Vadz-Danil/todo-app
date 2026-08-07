import React, { useRef, useState } from 'react';
import { Plus, X } from 'lucide-react';
import type { SubtaskProgress, Task, TaskInput, TaskStatus } from '../../types';
import { STATUS_LABELS } from '../../types';
import { STATUS_DOT, TaskCard } from '../TaskCard';
import { TaskCreate } from '../TaskCreate';

export interface KanbanColumnProps {
    status: TaskStatus;
    /** Already sorted by `position` ascending by the board. */
    tasks: Task[];
    /** Checklist rollups keyed by task id, loaded once by the board. */
    progress?: Record<string, SubtaskProgress>;
    loading?: boolean;
    draggingId: string | null;
    /** Insertion index within the rendered list, or null when this is not the hovered column. */
    dropIndex: number | null;
    onDragOverColumn: (status: TaskStatus, index: number) => void;
    onDragLeaveColumn: (status: TaskStatus) => void;
    onDropColumn: (status: TaskStatus, index: number) => void;
    onCardDragStart: (task: Task) => void;
    onCardDragEnd: () => void;
    onStatusChange: (task: Task, status: TaskStatus) => void;
    onOpen: (task: Task) => void;
    onDelete?: (id: string) => void;
    onCreate: (input: TaskInput) => Promise<void>;
}

const DropLine: React.FC = () => (
    <div
        aria-hidden="true"
        className="h-0.5 w-full shrink-0 rounded-full bg-accent ring-2 ring-accent/25"
    />
);

const CardSkeleton: React.FC = () => (
    <div className="animate-pulse rounded-2xl border border-border bg-surface p-4">
        <div className="h-3 w-16 rounded bg-hover" />
        <div className="mt-3 h-3.5 w-4/5 rounded bg-hover" />
        <div className="mt-2 h-3 w-3/5 rounded bg-hover" />
    </div>
);

export const KanbanColumn: React.FC<KanbanColumnProps> = ({
    status,
    tasks,
    progress,
    loading = false,
    draggingId,
    dropIndex,
    onDragOverColumn,
    onDragLeaveColumn,
    onDropColumn,
    onCardDragStart,
    onCardDragEnd,
    onStatusChange,
    onOpen,
    onDelete,
    onCreate,
}) => {
    const listRef = useRef<HTMLDivElement | null>(null);
    const [adding, setAdding] = useState(false);

    const cards = Array.isArray(tasks) ? tasks : [];
    const dragIndex = draggingId ? cards.findIndex((t) => t.id === draggingId) : -1;

    /** Insertion index from the pointer y against each rendered card's midpoint. */
    const indexFromPointer = (clientY: number): number => {
        const container = listRef.current;
        if (!container) return cards.length;
        const nodes = Array.from(container.querySelectorAll<HTMLElement>('[data-card-id]'));
        for (let i = 0; i < nodes.length; i += 1) {
            const rect = nodes[i].getBoundingClientRect();
            if (clientY < rect.top + rect.height / 2) return i;
        }
        return nodes.length;
    };

    const handleDragOver = (e: React.DragEvent<HTMLDivElement>) => {
        if (!draggingId) return;
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        onDragOverColumn(status, indexFromPointer(e.clientY));
    };

    const handleDragLeave = (e: React.DragEvent<HTMLDivElement>) => {
        const next = e.relatedTarget;
        if (next instanceof Node && e.currentTarget.contains(next)) return;
        onDragLeaveColumn(status);
    };

    const handleDrop = (e: React.DragEvent<HTMLDivElement>) => {
        if (!draggingId) return;
        e.preventDefault();
        onDropColumn(status, indexFromPointer(e.clientY));
    };

    // A line directly above or below the dragged card itself would be a no-op.
    const showLineAt = (index: number): boolean =>
        dropIndex === index && !(dragIndex !== -1 && (index === dragIndex || index === dragIndex + 1));

    const isDropTarget = dropIndex !== null && draggingId !== null;

    return (
        <section
            aria-label={`${STATUS_LABELS[status]}, ${cards.length} tasks`}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            className={`flex min-w-[280px] shrink-0 grow basis-[280px] flex-col rounded-2xl border transition-colors ${
                isDropTarget ? 'border-accent bg-accent-bg' : 'border-border bg-bg'
            }`}
        >
            <header className="flex items-center gap-2 border-b border-border px-3 py-2.5">
                <span aria-hidden="true" className={`h-2 w-2 shrink-0 rounded-full ${STATUS_DOT[status]}`} />
                <h3 className="min-w-0 flex-1 truncate text-[11px] font-mono font-semibold uppercase tracking-wider text-text-h">
                    {STATUS_LABELS[status]}
                </h3>
                <span className="shrink-0 rounded-lg border border-border bg-hover px-1.5 py-0.5 text-[10px] font-mono text-text">
                    {cards.length}
                </span>
                <button
                    type="button"
                    onClick={() => setAdding((v) => !v)}
                    aria-expanded={adding}
                    aria-label={adding ? `Cancel new task in ${STATUS_LABELS[status]}` : `Add a task to ${STATUS_LABELS[status]}`}
                    className="flex shrink-0 items-center gap-1 rounded-lg border border-border bg-hover px-1.5 py-0.5 text-[10px] font-mono font-semibold uppercase tracking-wider text-text transition hover:text-text-h active:scale-95"
                >
                    {adding ? <X className="h-3 w-3" /> : <Plus className="h-3 w-3" strokeWidth={3} />}
                    <span>{adding ? 'Close' : 'Add'}</span>
                </button>
            </header>

            <div
                ref={listRef}
                className="flex max-h-[calc(100svh-15rem)] min-h-[7rem] flex-1 flex-col gap-2.5 overflow-y-auto overflow-x-hidden p-2.5"
            >
                {adding && (
                    <TaskCreate
                        onCreate={onCreate}
                        defaultStatus={status}
                        autoFocus
                        onCancel={() => setAdding(false)}
                    />
                )}

                {loading && cards.length === 0 && (
                    <>
                        <CardSkeleton />
                        <CardSkeleton />
                    </>
                )}

                {!loading && cards.length === 0 && !adding && (
                    <div className="flex flex-1 flex-col items-center justify-center gap-1 rounded-xl border border-dashed border-border px-3 py-6 text-center">
                        <p className="text-xs font-medium text-text-h">Nothing here yet</p>
                        <p className="text-[11px] text-text">Drop a card or press Add.</p>
                    </div>
                )}

                {cards.map((task, i) => (
                    <React.Fragment key={task.id}>
                        {showLineAt(i) && <DropLine />}
                        <TaskCard
                            task={task}
                            progress={progress?.[task.id]}
                            draggable
                            isDragging={draggingId === task.id}
                            onDragStart={() => onCardDragStart(task)}
                            onDragEnd={onCardDragEnd}
                            onStatusChange={onStatusChange}
                            onOpen={onOpen}
                            onDelete={onDelete}
                        />
                    </React.Fragment>
                ))}

                {showLineAt(cards.length) && <DropLine />}
            </div>
        </section>
    );
};

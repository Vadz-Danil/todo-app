import React from 'react';
import { Check, Clock, CircleDot } from 'lucide-react';
import type { Task, TaskStatus } from '../types';

interface TaskCardProps {
    task: Task;
    onStatusChange: (id: string, status: TaskStatus) => void;
}

const statusStyles: Record<
    TaskStatus,
    { label: string; badgeClass: string; cardGlow: string }
> = {
    TODO: {
        label: 'To Do',
        badgeClass: 'bg-hover text-text border-border',
        cardGlow: 'hover:border-text-h/20',
    },
    IN_PROGRESS: {
        label: 'In Progress',
        badgeClass: 'bg-warning/10 text-warning border-warning/30',
        cardGlow: 'hover:border-warning/40',
    },
    DONE: {
        label: 'Done',
        badgeClass: 'bg-success/10 text-success border-success/30',
        cardGlow: 'hover:border-success/40',
    },
};

export const TaskCard: React.FC<TaskCardProps> = ({ task, onStatusChange }) => {
    const config = statusStyles[task.status] || statusStyles.TODO;

    return (
        <div
            className={`group relative flex flex-col justify-between rounded-2xl border border-border bg-surface p-4 sm:p-5 transition-all duration-300 shadow-(--shadow) hover:bg-hover ${config.cardGlow}`}
        >
            <div>
                <div className="flex items-center justify-between gap-2">
                    <span
                        className={`inline-flex items-center rounded-lg border px-2.5 py-0.5 text-[10px] font-mono font-medium uppercase tracking-wider ${config.badgeClass}`}
                    >
                        {config.label}
                    </span>

                    <button
                        onClick={() =>
                            onStatusChange(task.id, task.status === 'DONE' ? 'TODO' : 'DONE')
                        }
                        className="rounded-lg p-1 text-text hover:text-text-h transition"
                    >
                        {task.status === 'DONE' ? (
                            <div className="flex h-5 w-5 items-center justify-center rounded-md bg-success text-bg">
                                <Check className="h-3.5 w-3.5" strokeWidth={3} />
                            </div>
                        ) : task.status === 'IN_PROGRESS' ? (
                            <Clock className="h-5 w-5 text-warning" />
                        ) : (
                            <CircleDot className="h-5 w-5 text-text/60" />
                        )}
                    </button>
                </div>

                <h3
                    className={`mt-3 text-sm font-semibold text-text-h leading-snug transition ${
                        task.status === 'DONE' ? 'line-through text-text/60' : ''
                    }`}
                >
                    {task.title}
                </h3>

                {task.description && (
                    <p className="mt-1.5 text-xs text-text line-clamp-3 leading-relaxed">
                        {task.description}
                    </p>
                )}
            </div>

            <div className="mt-4 flex items-center justify-between border-t border-border/60 pt-3">
                <span className="text-[10px] font-mono text-text/60 uppercase">Change status</span>
                <select
                    value={task.status}
                    onChange={(e) => onStatusChange(task.id, e.target.value as TaskStatus)}
                    className="rounded-lg border bg-bg px-2 py-1 text-xs font-mono text-text-h outline-none transition focus:border-success"
                >
                    <option value="TODO">To Do</option>
                    <option value="IN_PROGRESS">In Progress</option>
                    <option value="DONE">Done</option>
                </select>
            </div>
        </div>
    );
};
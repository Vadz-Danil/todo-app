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
        badgeClass: 'bg-zinc-800/80 text-zinc-300 border-zinc-700',
        cardGlow: 'hover:border-zinc-700',
    },
    IN_PROGRESS: {
        label: 'In Progress',
        badgeClass: 'bg-amber-500/10 text-amber-400 border-amber-500/30',
        cardGlow: 'hover:border-amber-500/40',
    },
    DONE: {
        label: 'Done',
        badgeClass: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/30',
        cardGlow: 'hover:border-emerald-500/40',
    },
};

export const TaskCard: React.FC<TaskCardProps> = ({ task, onStatusChange }) => {
    const config = statusStyles[task.status] || statusStyles.TODO;

    return (
        <div
            className={`group relative flex flex-col justify-between rounded-2xl border border-zinc-800/80 bg-zinc-900/40 p-4 sm:p-5 transition-all duration-300 ${config.cardGlow} hover:bg-zinc-900/80`}
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
                        className="rounded-lg p-1 text-zinc-500 hover:text-zinc-200 transition"
                    >
                        {task.status === 'DONE' ? (
                            <div className="flex h-5 w-5 items-center justify-center rounded-md bg-emerald-500 text-zinc-950">
                                <Check className="h-3.5 w-3.5 stroke-3" />
                            </div>
                        ) : task.status === 'IN_PROGRESS' ? (
                            <Clock className="h-5 w-5 text-amber-400" />
                        ) : (
                            <CircleDot className="h-5 w-5 text-zinc-600" />
                        )}
                    </button>
                </div>

                <h3
                    className={`mt-3 text-sm font-semibold text-zinc-100 leading-snug transition ${
                        task.status === 'DONE' ? 'line-through text-zinc-500' : ''
                    }`}
                >
                    {task.title}
                </h3>

                {task.description && (
                    <p className="mt-1.5 text-xs text-zinc-400 line-clamp-3 leading-relaxed">
                        {task.description}
                    </p>
                )}
            </div>

            <div className="mt-4 flex items-center justify-between border-t border-zinc-800/60 pt-3">
                <span className="text-[10px] font-mono text-zinc-500 uppercase">Change status</span>
                <select
                    value={task.status}
                    onChange={(e) => onStatusChange(task.id, e.target.value as TaskStatus)}
                    className="rounded-lg border border-zinc-800 bg-zinc-950 px-2 py-1 text-xs font-mono text-zinc-300 outline-none transition focus:ring-1 focus:ring-emerald-500"
                >
                    <option value="TODO">To Do</option>
                    <option value="IN_PROGRESS">In Progress</option>
                    <option value="DONE">Done</option>
                </select>
            </div>
        </div>
    );
};
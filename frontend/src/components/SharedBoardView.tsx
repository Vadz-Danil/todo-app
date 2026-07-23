import React, { useEffect, useState } from 'react';
import { Clock, Eye, Link2Off, Loader2 } from 'lucide-react';
import { fetchSharedView } from '../api/endpoints';
import {
    BOARD_STATUSES,
    PRIORITY_LABELS,
    STATUS_LABELS,
    type SharedTask,
    type SharedView,
} from '../types';

interface SharedBoardViewProps {
    token: string;
}

const PRIORITY_STYLES: Record<string, string> = {
    URGENT: 'border-red-500/40 text-red-400',
    HIGH: 'border-amber-500/40 text-amber-400',
    MEDIUM: 'border-sky-500/40 text-sky-400',
    LOW: 'border-zinc-600 text-zinc-400',
};

const formatDate = (iso: string): string =>
    new Date(iso).toLocaleDateString(undefined, { day: 'numeric', month: 'short' });

const TaskCard: React.FC<{ task: SharedTask }> = ({ task }) => (
    <article className="rounded-xl border border-border bg-surface p-3 shadow-(--shadow)">
        <h4 className="text-sm font-semibold leading-snug text-text-h">{task.title}</h4>

        {task.description && (
            <p className="mt-1.5 line-clamp-3 text-xs leading-relaxed text-text">{task.description}</p>
        )}

        <div className="mt-2.5 flex flex-wrap items-center gap-1.5">
            <span
                className={`rounded-md border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wide ${
                    PRIORITY_STYLES[task.priority] ?? PRIORITY_STYLES.LOW
                }`}
            >
                {PRIORITY_LABELS[task.priority] ?? task.priority}
            </span>

            {task.reviewer && (
                <span className="rounded-md border border-border px-1.5 py-0.5 text-[10px] text-text">
                    review: {task.reviewer}
                </span>
            )}

            {task.due_date && (
                <span className="flex items-center gap-1 rounded-md border border-border px-1.5 py-0.5 text-[10px] text-text">
                    <Clock className="h-2.5 w-2.5" />
                    {formatDate(task.due_date)}
                </span>
            )}

            {task.estimate_hours != null && (
                <span className="rounded-md border border-border px-1.5 py-0.5 text-[10px] text-text">
                    {task.estimate_hours}h
                </span>
            )}
        </div>

        {task.blockers && (
            <p className="mt-2 rounded-lg border border-red-500/20 bg-red-500/5 px-2 py-1 text-[11px] text-red-400">
                Blocked: {task.blockers}
            </p>
        )}
    </article>
);

export const SharedBoardView: React.FC<SharedBoardViewProps> = ({ token }) => {
    const [view, setView] = useState<SharedView | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;

        fetchSharedView(token)
            .then((data) => {
                if (!cancelled) setView(data);
            })
            .catch((err) => {
                if (cancelled) return;
                const status = (err as { response?: { status?: number } })?.response?.status;
                setError(
                    status === 404
                        ? 'This link is no longer available. It may have been revoked or has expired.'
                        : 'Could not load this board. Please try again later.'
                );
            })
            .finally(() => {
                if (!cancelled) setLoading(false);
            });

        return () => {
            cancelled = true;
        };
    }, [token]);

    if (loading) {
        return (
            <div className="flex min-h-screen items-center justify-center bg-bg">
                <Loader2 className="h-6 w-6 animate-spin text-text" />
            </div>
        );
    }

    if (error || !view) {
        return (
            <div className="flex min-h-screen items-center justify-center bg-bg px-6">
                <div className="max-w-md text-center">
                    <div className="mx-auto mb-4 w-fit rounded-2xl border border-border bg-surface p-4 text-text">
                        <Link2Off className="h-6 w-6" />
                    </div>
                    <h1 className="text-xl font-bold text-text-h">Link unavailable</h1>
                    <p className="mt-2 text-sm leading-relaxed text-text">{error}</p>
                </div>
            </div>
        );
    }

    const totals = view.analytics?.totals;

    return (
        <div className="min-h-screen bg-bg text-text-h antialiased">
            <header className="border-b border-border bg-surface/60 backdrop-blur-md">
                <div className="mx-auto flex w-full max-w-[1600px] flex-wrap items-center justify-between gap-3 px-4 py-4 sm:px-6">
                    <div className="min-w-0">
                        <div className="flex items-center gap-2">
                            <span className="font-mono text-xs font-bold tracking-wider text-text-h">
                                TASK//FLOW
                            </span>
                            <span className="rounded-md border border-border px-1.5 py-0.5 font-mono text-[10px] uppercase tracking-wider text-text">
                                Read-only
                            </span>
                        </div>
                        <h1 className="mt-1 truncate text-sm text-text">
                            {view.label || 'Shared board'} · {view.owner_email}
                        </h1>
                    </div>

                    <div className="flex items-center gap-3 text-[11px] text-text">
                        <span className="flex items-center gap-1">
                            <Eye className="h-3 w-3" />
                            {view.task_count} task{view.task_count === 1 ? '' : 's'}
                        </span>
                        {view.expires_at && <span>expires {formatDate(view.expires_at)}</span>}
                    </div>
                </div>
            </header>

            <main className="mx-auto w-full max-w-[1600px] px-4 py-6 sm:px-6 lg:py-8">
                {totals && (
                    <section className="mb-6 grid grid-cols-2 gap-3 sm:grid-cols-4">
                        {[
                            { label: 'Completed', value: totals.completed_in_range },
                            { label: 'Created', value: totals.created_in_range },
                            { label: 'Open now', value: totals.open_now },
                            {
                                label: 'Completion',
                                value: `${Math.round(totals.completion_rate * 100)}%`,
                            },
                        ].map((stat) => (
                            <div
                                key={stat.label}
                                className="rounded-2xl border border-border bg-surface p-4 shadow-(--shadow)"
                            >
                                <p className="font-mono text-[10px] uppercase tracking-wider text-text">
                                    {stat.label}
                                </p>
                                <p className="mt-1 text-2xl font-bold text-text-h">{stat.value}</p>
                            </div>
                        ))}
                    </section>
                )}

                {view.columns && (
                    <section className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-4">
                        {BOARD_STATUSES.map((status) => {
                            const column = view.columns?.find((c) => c.status === status);
                            const tasks = column?.tasks ?? [];

                            return (
                                <div key={status} className="flex flex-col gap-3">
                                    <div className="flex items-center justify-between px-1">
                                        <h2 className="font-mono text-xs font-bold uppercase tracking-wider text-text-h">
                                            {STATUS_LABELS[status]}
                                        </h2>
                                        <span className="rounded-md border border-border px-1.5 py-0.5 font-mono text-[10px] text-text">
                                            {tasks.length}
                                        </span>
                                    </div>

                                    {tasks.length === 0 ? (
                                        <p className="rounded-xl border border-dashed border-border px-3 py-6 text-center text-[11px] text-text">
                                            Nothing here
                                        </p>
                                    ) : (
                                        tasks.map((task) => <TaskCard key={task.id} task={task} />)
                                    )}
                                </div>
                            );
                        })}
                    </section>
                )}
            </main>

            <footer className="border-t border-border px-4 py-6 text-center text-[11px] text-text sm:px-6">
                Shared from TaskFlow · this page is read-only and does not update live
            </footer>
        </div>
    );
};

import React, { useId, useState } from 'react';
import { ChartColumn, Table } from 'lucide-react';

export interface ChartCardProps {
    title: string;
    subtitle?: string;
    /** Rendered under the header in chart view only. */
    legend?: React.ReactNode;
    /** The same numbers as an accessible table — mandatory, never optional. */
    table: React.ReactNode;
    children: React.ReactNode;
    className?: string;
    /** Refetch in flight: hold the previous render at reduced opacity. */
    loading?: boolean;
}

type CardView = 'chart' | 'table';

const TOGGLE: { id: CardView; label: string; icon: React.ComponentType<{ className?: string }> }[] = [
    { id: 'chart', label: 'Chart', icon: ChartColumn },
    { id: 'table', label: 'Table', icon: Table },
];

export const ChartCard: React.FC<ChartCardProps> = ({
    title,
    subtitle,
    legend,
    table,
    children,
    className = '',
    loading = false,
}) => {
    const [view, setView] = useState<CardView>('chart');
    const bodyId = useId();

    return (
        <section
            className={`flex min-w-0 flex-col rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) sm:p-5 ${className}`}
            aria-busy={loading || undefined}
        >
            <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
                <div className="min-w-0">
                    <h3 className="truncate text-xs font-mono font-medium uppercase tracking-wider text-text">
                        {title}
                    </h3>
                    {subtitle && <p className="mt-1 text-xs text-text/80">{subtitle}</p>}
                </div>

                <div
                    role="group"
                    aria-label={`${title} view`}
                    className="flex shrink-0 items-center gap-0.5 rounded-xl border border-border bg-hover/60 p-0.5"
                >
                    {TOGGLE.map((t) => {
                        const Icon = t.icon;
                        const active = view === t.id;
                        return (
                            <button
                                key={t.id}
                                type="button"
                                onClick={() => setView(t.id)}
                                aria-pressed={active}
                                aria-controls={bodyId}
                                className={`flex items-center gap-1 rounded-lg px-2 py-1 text-[10px] font-mono font-medium uppercase tracking-wider transition active:scale-95 ${
                                    active
                                        ? 'bg-surface text-text-h shadow-sm'
                                        : 'text-text hover:text-text-h'
                                }`}
                            >
                                <Icon className="h-3.5 w-3.5" />
                                <span className="hidden sm:inline">{t.label}</span>
                            </button>
                        );
                    })}
                </div>
            </div>

            {view === 'chart' && legend && <div className="mt-3">{legend}</div>}

            <div
                id={bodyId}
                className={`mt-3 min-w-0 flex-1 transition-opacity duration-200 ${
                    loading ? 'opacity-50' : 'opacity-100'
                }`}
            >
                {view === 'chart' ? children : table}
            </div>
        </section>
    );
};

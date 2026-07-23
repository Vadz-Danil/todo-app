import React from 'react';
import { ArrowDownRight, ArrowUpRight, Minus } from 'lucide-react';
import { VIZ } from './primitives';

export interface StatDelta {
    /** Signed fraction from the API's `change_pct` (0.25 == +25%). */
    changePct: number;
    /** Direction of good, already resolved by the caller (cycle time is inverted). */
    improved: boolean;
}

export interface StatTileProps {
    label: string;
    value: string;
    /** The one hero number on the page: >= 48px, system sans, proportional figures. */
    hero?: boolean;
    delta?: StatDelta;
    deltaHint?: string;
    footnote?: string;
    icon?: React.ComponentType<{ className?: string }>;
    className?: string;
}

export const StatTile: React.FC<StatTileProps> = ({
    label,
    value,
    hero = false,
    delta,
    deltaHint,
    footnote,
    icon: Icon,
    className = '',
}) => {
    const flat = delta !== undefined && Math.abs(delta.changePct) < 0.0005;
    const Arrow = flat ? Minus : delta && delta.changePct > 0 ? ArrowUpRight : ArrowDownRight;
    const deltaColor = flat ? VIZ.muted : delta?.improved ? VIZ.good : VIZ.critical;

    return (
        <div
            className={`flex min-w-0 flex-col justify-between rounded-2xl border border-border bg-surface p-4 shadow-(--shadow) ${
                hero ? 'sm:p-5' : ''
            } ${className}`}
        >
            <div className="flex items-center gap-1.5">
                {Icon && <Icon className="h-3.5 w-3.5 shrink-0 text-text/70" />}
                <span className="truncate text-[10px] font-mono font-medium uppercase tracking-wider text-text">
                    {label}
                </span>
            </div>

            {/* Proportional figures: tabular-nums makes a big number look loose. */}
            <div
                className={`mt-2 font-semibold leading-none text-text-h ${
                    hero ? 'text-[48px] sm:text-[56px]' : 'text-2xl sm:text-3xl'
                }`}
            >
                {value}
            </div>

            <div className="mt-2 flex min-h-4 flex-wrap items-center gap-x-1.5 gap-y-0.5">
                {/* A tile with no comparison shows nothing at all — never "0%". */}
                {delta && (
                    <span className="flex items-center gap-0.5 text-xs font-semibold" style={{ color: deltaColor }}>
                        <Arrow className="h-3.5 w-3.5" />
                        {`${Math.abs(delta.changePct * 100).toFixed(0)}%`}
                    </span>
                )}
                {delta && deltaHint && (
                    <span className="text-[10px]" style={{ color: VIZ.muted }}>
                        {deltaHint}
                    </span>
                )}
                {!delta && footnote && (
                    <span className="text-[10px]" style={{ color: VIZ.muted }}>
                        {footnote}
                    </span>
                )}
            </div>
        </div>
    );
};

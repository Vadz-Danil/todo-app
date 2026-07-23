import React from 'react';
import { VIZ } from './primitives';

export interface MeterProps {
    label: string;
    /** Secondary line under the name, e.g. the sprint's date range. */
    sublabel?: string;
    /** 0..1 — the API sends every rate as a fraction. */
    value: number;
    /** Right-hand readout under the bar, e.g. "12 / 20 done". */
    valueLabel?: string;
    className?: string;
}

export const Meter: React.FC<MeterProps> = ({ label, sublabel, value, valueLabel, className = '' }) => {
    const pct = Math.max(0, Math.min(1, Number.isFinite(value) ? value : 0));
    const percentText = `${Math.round(pct * 100)}%`;

    return (
        <div className={`min-w-0 ${className}`}>
            <div className="flex items-baseline justify-between gap-3">
                <span className="truncate text-sm font-medium text-text-h">{label}</span>
                <span className="shrink-0 text-sm font-semibold text-text-h">{percentText}</span>
            </div>

            {(sublabel || valueLabel) && (
                <div className="mt-0.5 flex items-baseline justify-between gap-3 text-[11px]">
                    <span className="truncate" style={{ color: VIZ.muted }}>
                        {sublabel}
                    </span>
                    <span className="shrink-0 text-text">{valueLabel}</span>
                </div>
            )}

            {/* Fill and track are two steps of the same ramp, so state reads
                across the whole bar rather than only where it is filled. */}
            <div
                className="mt-2 h-2.5 w-full overflow-hidden rounded-[4px]"
                role="meter"
                aria-valuenow={Math.round(pct * 100)}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-label={`${label} completion`}
                style={{ background: 'var(--viz-seq-100)' }}
            >
                <div
                    className="h-full rounded-[4px] transition-[width] duration-300"
                    style={{ width: `${pct * 100}%`, background: 'var(--viz-seq-500)' }}
                />
            </div>
        </div>
    );
};

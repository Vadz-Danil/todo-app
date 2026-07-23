import React, { useState } from 'react';
import type { DayCount } from '../../types';
import { ChartMessage, ChartTooltip, VIZ, safeArray, useElementWidth, type TooltipState } from './primitives';

/** Five bins off the one sequential hue, light -> dark. Never a rainbow. */
export const HEAT_BINS = [VIZ.seq[0], VIZ.seq[1], VIZ.seq[3], VIZ.seq[5], VIZ.seq[6]];

const MIN_PITCH = 18; // Never below this: 18px + the 6px gap keeps a legal hit target.
const MAX_PITCH = 34;
const DEFAULT_PITCH = 24;
const ROW_LABEL_W = 32;
const TOP_BAND = 18;
const WEEKDAYS = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

/** `YYYY-MM-DD` is a local calendar day — never let Date parse it as UTC. */
const parseLocalDate = (key: string): Date | null => {
    const m = /^(\d{4})-(\d{2})-(\d{2})/.exec(key);
    if (!m) return null;
    const d = new Date(Number(m[1]), Number(m[2]) - 1, Number(m[3]));
    return Number.isNaN(d.getTime()) ? null : d;
};

const mondayIndex = (d: Date): number => (d.getDay() + 6) % 7;

const dayKey = (d: Date): string =>
    `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

export interface HeatmapProps {
    days: DayCount[];
    ariaLabel: string;
    emptyMessage?: string;
}

interface Cell {
    key: string;
    col: number;
    row: number;
    count: number;
    date: Date;
}

export const Heatmap: React.FC<HeatmapProps> = ({
    days,
    ariaLabel,
    emptyMessage = 'No completed days in this range.',
}) => {
    const [hover, setHover] = useState<Cell | null>(null);
    const [ref, avail] = useElementWidth();

    const source = safeArray(days)
        .map((d) => ({ date: parseLocalDate(d.date), count: Math.max(0, d.count || 0) }))
        .filter((d): d is { date: Date; count: number } => d.date !== null)
        .sort((a, b) => a.date.getTime() - b.date.getTime());

    if (source.length === 0) return <ChartMessage height={150}>{emptyMessage}</ChartMessage>;

    const counts = new Map(source.map((d) => [dayKey(d.date), d.count]));
    const first = source[0].date;
    const last = source[source.length - 1].date;

    // Whole weeks: Monday on/before the first day .. Sunday on/after the last.
    const start = new Date(first.getFullYear(), first.getMonth(), first.getDate() - mondayIndex(first));
    const end = new Date(last.getFullYear(), last.getMonth(), last.getDate() + (6 - mondayIndex(last)));

    const cells: Cell[] = [];
    const monthTicks: { col: number; label: string }[] = [];
    let lastMonth = -1;
    let lastMonthCol = -99;

    for (
        let cursor = new Date(start), i = 0;
        cursor.getTime() <= end.getTime();
        i++, cursor = new Date(cursor.getFullYear(), cursor.getMonth(), cursor.getDate() + 1)
    ) {
        const col = Math.floor(i / 7);
        const row = i % 7;
        const key = dayKey(cursor);

        if (row === 0 && cursor.getMonth() !== lastMonth && col - lastMonthCol >= 3) {
            monthTicks.push({ col, label: cursor.toLocaleDateString(undefined, { month: 'short' }) });
            lastMonth = cursor.getMonth();
            lastMonthCol = col;
        }

        const count = counts.get(key);
        if (count !== undefined) cells.push({ key, col, row, count, date: new Date(cursor) });
    }

    const weeks = cells.reduce((m, c) => Math.max(m, c.col + 1), 1);
    const max = source.reduce((m, d) => Math.max(m, d.count), 0);
    // Grow the cells to use the card's width for short ranges; long ranges
    // fall back to the minimum pitch and scroll inside the card.
    const PITCH =
        avail > 0
            ? Math.max(MIN_PITCH, Math.min(MAX_PITCH, Math.floor((avail - ROW_LABEL_W) / weeks)))
            : DEFAULT_PITCH;
    const CELL = PITCH - 4;
    const width = ROW_LABEL_W + weeks * PITCH;
    const height = TOP_BAND + 7 * PITCH;

    const binOf = (count: number): string =>
        count <= 0 || max <= 0
            ? 'var(--hover)'
            : HEAT_BINS[Math.min(HEAT_BINS.length - 1, Math.ceil((count / max) * HEAT_BINS.length) - 1)];

    const tip: TooltipState | null = hover
        ? {
              x: ROW_LABEL_W + hover.col * PITCH + CELL / 2,
              y: TOP_BAND + hover.row * PITCH + CELL,
              below: true,
              title: hover.date.toLocaleDateString(undefined, {
                  weekday: 'short',
                  day: 'numeric',
                  month: 'short',
              }),
              rows: [
                  {
                      label: 'Completed',
                      value: String(hover.count),
                      color: binOf(hover.count),
                  },
              ],
          }
        : null;

    return (
        // Long ranges scroll here, inside the card — the page body never does.
        <div ref={ref} className="overflow-x-auto pb-14">
            <div className="relative" style={{ width }} onPointerLeave={() => setHover(null)}>
                <svg role="img" aria-label={ariaLabel} width={width} height={height} viewBox={`0 0 ${width} ${height}`}>
                    {monthTicks.map((t) => (
                        <text
                            key={`${t.label}-${t.col}`}
                            x={ROW_LABEL_W + t.col * PITCH}
                            y={11}
                            fontSize={10}
                            fill={VIZ.muted}
                        >
                            {t.label}
                        </text>
                    ))}

                    {WEEKDAYS.map((w, row) => (
                        <text
                            key={w}
                            x={ROW_LABEL_W - 8}
                            y={TOP_BAND + row * PITCH + CELL / 2 + 3}
                            textAnchor="end"
                            fontSize={10}
                            fill={VIZ.muted}
                        >
                            {w}
                        </text>
                    ))}

                    {cells.map((c) => (
                        <g key={c.key}>
                            <rect
                                x={ROW_LABEL_W + c.col * PITCH}
                                y={TOP_BAND + c.row * PITCH}
                                width={CELL}
                                height={CELL}
                                rx={4}
                                fill={binOf(c.count)}
                            />
                            <rect
                                x={ROW_LABEL_W + c.col * PITCH}
                                y={TOP_BAND + c.row * PITCH}
                                width={PITCH}
                                height={PITCH}
                                fill="transparent"
                                onPointerEnter={() => setHover(c)}
                            />
                        </g>
                    ))}
                </svg>
                <ChartTooltip tip={tip} width={width} />
            </div>
        </div>
    );
};

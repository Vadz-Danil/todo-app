import React, { useState } from 'react';
import {
    ChartGrid,
    ChartMessage,
    ChartTooltip,
    VIZ,
    XTicks,
    estTextWidth,
    fmtNum,
    linearTicks,
    localPoint,
    makeFrame,
    safeArray,
    useElementWidth,
    type TooltipState,
} from './primitives';

export interface LineSeries {
    key: string;
    label: string;
    /** Follows the entity, never its rank — filtering must not repaint it. */
    color: string;
    /** null means "no measurement here" and breaks the line, rather than plotting a zero. */
    values: (number | null)[];
}

export interface LineChartProps {
    labels: string[];
    series: LineSeries[];
    /** Fixed, and it already includes the x-axis label band. */
    height?: number;
    format?: (n: number) => string;
    ariaLabel: string;
    emptyMessage?: string;
}

interface EndLabel {
    key: string;
    text: string;
    y: number;
    ly: number;
    at: number;
}

export const LineChart: React.FC<LineChartProps> = ({
    labels,
    series,
    height = 250,
    format = fmtNum,
    ariaLabel,
    emptyMessage = 'No activity in this range.',
}) => {
    const [ref, width] = useElementWidth();
    const [hover, setHover] = useState<number | null>(null);

    const rows = safeArray(labels);
    const lines = safeArray(series).filter((s) => safeArray(s.values).length > 0);
    const n = rows.length;

    const max = lines.reduce(
        (acc, s) => s.values.reduce<number>((m, v) => (Number.isFinite(v) && (v as number) > m ? (v as number) : m), acc),
        0
    );
    const ticks = linearTicks(max, 4, Number.isInteger(max));
    const top = ticks[ticks.length - 1];

    // Endpoint annotations describe the last bucket that actually has a value.
    const lastIndex = (s: LineSeries) => {
        for (let i = n - 1; i >= 0; i--) if (Number.isFinite(s.values[i])) return i;
        return -1;
    };
    const endIndex = lines.map(lastIndex);
    const endText = lines.map((s, i) => (endIndex[i] < 0 ? '' : format(s.values[endIndex[i]] as number)));
    const endWidth = endText.reduce((acc, t) => Math.max(acc, estTextWidth(t, 11)), 0);

    const f = makeFrame(width, height, {
        top: 14,
        right: Math.min(86, Math.max(46, endWidth + 20)),
        bottom: 34,
        left: 42,
    });

    const xFor = (i: number) => (n <= 1 ? f.m.left + f.iw / 2 : f.m.left + (i / (n - 1)) * f.iw);
    const yFor = (v: number) =>
        f.m.top + f.ih - (top > 0 ? (Math.min(Math.max(0, v), top) / top) * f.ih : 0);

    // Endpoint labels: nudge apart only as far as they must, then own the
    // displacement with a leader line rather than floating free of the line.
    const endLabels: EndLabel[] = lines
        .map((s, i) => {
            if (endIndex[i] < 0) return null;
            const y = yFor(s.values[endIndex[i]] as number);
            return { key: s.key, text: endText[i], y, ly: y, at: endIndex[i] };
        })
        .filter((l): l is EndLabel => l !== null);
    const stacked = [...endLabels].sort((a, b) => a.y - b.y);
    for (let i = 1; i < stacked.length; i++) {
        if (stacked[i].ly - stacked[i - 1].ly < 14) stacked[i].ly = stacked[i - 1].ly + 14;
    }
    if (stacked.length > 0) {
        const overflow = stacked[stacked.length - 1].ly - (f.m.top + f.ih);
        if (overflow > 0) stacked.forEach((s) => (s.ly -= overflow));
    }

    const onMove = (e: React.PointerEvent<SVGSVGElement>) => {
        if (f.iw <= 0) return;
        const { x } = localPoint(e);
        if (x < f.m.left - 12 || x > f.m.left + f.iw + 12) {
            setHover(null);
            return;
        }
        const t = n <= 1 ? 0 : (x - f.m.left) / f.iw;
        setHover(Math.max(0, Math.min(n - 1, Math.round(t * (n - 1)))));
    };

    const tip: TooltipState | null =
        hover === null || width === 0
            ? null
            : {
                  x: xFor(hover),
                  y: Math.min(
                      ...lines.map((s) =>
                          Number.isFinite(s.values[hover]) ? yFor(s.values[hover] as number) : f.m.top + f.ih
                      )
                  ),
                  title: rows[hover] ?? '',
                  rows: lines.map((s) => ({
                      label: s.label,
                      value: Number.isFinite(s.values[hover]) ? format(s.values[hover] as number) : 'no data',
                      color: s.color,
                  })),
              };

    if (n === 0 || lines.length === 0) {
        return <ChartMessage height={height - 60}>{emptyMessage}</ChartMessage>;
    }

    return (
        <div ref={ref} className="relative w-full" style={{ minHeight: height }}>
            {width > 0 && (
                <svg
                    role="img"
                    aria-label={ariaLabel}
                    width="100%"
                    height={height}
                    viewBox={`0 0 ${width} ${height}`}
                    onPointerMove={onMove}
                    onPointerLeave={() => setHover(null)}
                    style={{ touchAction: 'pan-y' }}
                >
                    <ChartGrid f={f} ticks={ticks} max={top} format={format} />
                    <XTicks f={f} labels={rows} xFor={xFor} />

                    {hover !== null && (
                        <line
                            x1={xFor(hover)}
                            x2={xFor(hover)}
                            y1={f.m.top}
                            y2={f.m.top + f.ih}
                            stroke={VIZ.axis}
                            strokeWidth={1}
                            shapeRendering="crispEdges"
                        />
                    )}

                    {/* A run of nulls breaks the path: a bucket with no sample is a
                        gap, not a drop to zero. */}
                    {lines.map((s) => (
                        <path
                            key={s.key}
                            d={rows
                                .map((_, i) =>
                                    Number.isFinite(s.values[i])
                                        ? `${Number.isFinite(s.values[i - 1]) && i > 0 ? 'L' : 'M'}${xFor(i)},${yFor(
                                              s.values[i] as number
                                          )}`
                                        : ''
                                )
                                .filter(Boolean)
                                .join(' ')}
                            fill="none"
                            stroke={s.color}
                            strokeWidth={2}
                            strokeLinecap="round"
                            strokeLinejoin="round"
                        />
                    ))}

                    {/* An isolated sample has no segment to sit on, so give it a dot. */}
                    {lines.flatMap((s) =>
                        rows
                            .map((_, i) =>
                                Number.isFinite(s.values[i]) &&
                                !Number.isFinite(s.values[i - 1]) &&
                                !Number.isFinite(s.values[i + 1]) ? (
                                    <circle
                                        key={`${s.key}-iso-${i}`}
                                        cx={xFor(i)}
                                        cy={yFor(s.values[i] as number)}
                                        r={3}
                                        fill={s.color}
                                    />
                                ) : null
                            )
                            .filter(Boolean)
                    )}

                    {/* End markers carry a 2px surface ring so crossings stay legible. */}
                    {endLabels.map((l) => (
                        <circle
                            key={`${l.key}-end`}
                            cx={xFor(l.at)}
                            cy={l.y}
                            r={4.5}
                            fill={lines.find((s) => s.key === l.key)?.color}
                            stroke={VIZ.surface}
                            strokeWidth={2}
                        />
                    ))}

                    {endLabels.map((l) => (
                        <g key={`${l.key}-label`}>
                            {Math.abs(l.ly - l.y) > 1.5 && (
                                <path
                                    d={`M${xFor(l.at) + 7},${l.y} L${xFor(l.at) + 11},${l.ly}`}
                                    stroke={VIZ.muted}
                                    strokeWidth={1}
                                    fill="none"
                                />
                            )}
                            <text x={xFor(l.at) + 13} y={l.ly + 4} fontSize={11} fontWeight={600} fill={VIZ.ink}>
                                {l.text}
                            </text>
                        </g>
                    ))}

                    {hover !== null &&
                        lines.map((s) =>
                            Number.isFinite(s.values[hover]) ? (
                                <circle
                                    key={`${s.key}-hover`}
                                    cx={xFor(hover)}
                                    cy={yFor(s.values[hover] as number)}
                                    r={4.5}
                                    fill={s.color}
                                    stroke={VIZ.surface}
                                    strokeWidth={2}
                                />
                            ) : null
                        )}
                </svg>
            )}
            <ChartTooltip tip={tip} width={width} />
        </div>
    );
};

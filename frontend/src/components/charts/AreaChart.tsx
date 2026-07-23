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

export interface AreaChartProps {
    labels: string[];
    values: number[];
    /** Single series — the card title names it, so there is no legend. */
    label: string;
    color?: string;
    height?: number;
    format?: (n: number) => string;
    ariaLabel: string;
    emptyMessage?: string;
}

export const AreaChart: React.FC<AreaChartProps> = ({
    labels,
    values,
    label,
    color = VIZ.cat[0],
    height = 230,
    format = fmtNum,
    ariaLabel,
    emptyMessage = 'No backlog history in this range.',
}) => {
    const [ref, width] = useElementWidth();
    const [hover, setHover] = useState<number | null>(null);

    const rows = safeArray(labels);
    const data = safeArray(values);
    const n = Math.min(rows.length, data.length);

    const max = data.slice(0, n).reduce((m, v) => (Number.isFinite(v) && v > m ? v : m), 0);
    const ticks = linearTicks(max, 4, Number.isInteger(max));
    const top = ticks[ticks.length - 1];

    const endText = format(data[n - 1] ?? 0);
    const f = makeFrame(width, height, {
        top: 14,
        right: Math.min(86, Math.max(46, estTextWidth(endText, 11) + 20)),
        bottom: 34,
        left: 42,
    });

    const xFor = (i: number) => (n <= 1 ? f.m.left + f.iw / 2 : f.m.left + (i / (n - 1)) * f.iw);
    const yFor = (v: number) =>
        f.m.top + f.ih - (top > 0 ? (Math.min(Math.max(0, v), top) / top) * f.ih : 0);

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
                  y: yFor(data[hover] ?? 0),
                  title: rows[hover] ?? '',
                  rows: [{ label, value: format(data[hover] ?? 0), color }],
              };

    if (n === 0) return <ChartMessage height={height - 60}>{emptyMessage}</ChartMessage>;

    const line = Array.from({ length: n }, (_, i) => `${i === 0 ? 'M' : 'L'}${xFor(i)},${yFor(data[i] ?? 0)}`).join(
        ' '
    );
    const baseline = f.m.top + f.ih;
    const area = `${line} L${xFor(n - 1)},${baseline} L${xFor(0)},${baseline} Z`;

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

                    <path d={area} fill={color} fillOpacity={0.12} stroke="none" />

                    {hover !== null && (
                        <line
                            x1={xFor(hover)}
                            x2={xFor(hover)}
                            y1={f.m.top}
                            y2={baseline}
                            stroke={VIZ.axis}
                            strokeWidth={1}
                            shapeRendering="crispEdges"
                        />
                    )}

                    <path d={line} fill="none" stroke={color} strokeWidth={2} strokeLinecap="round" strokeLinejoin="round" />

                    <circle
                        cx={xFor(n - 1)}
                        cy={yFor(data[n - 1] ?? 0)}
                        r={4.5}
                        fill={color}
                        stroke={VIZ.surface}
                        strokeWidth={2}
                    />
                    <text
                        x={xFor(n - 1) + 13}
                        y={yFor(data[n - 1] ?? 0) + 4}
                        fontSize={11}
                        fontWeight={600}
                        fill={VIZ.ink}
                    >
                        {endText}
                    </text>

                    {hover !== null && (
                        <circle
                            cx={xFor(hover)}
                            cy={yFor(data[hover] ?? 0)}
                            r={4.5}
                            fill={color}
                            stroke={VIZ.surface}
                            strokeWidth={2}
                        />
                    )}
                </svg>
            )}
            <ChartTooltip tip={tip} width={width} />
        </div>
    );
};

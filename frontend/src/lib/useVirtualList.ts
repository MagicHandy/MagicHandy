import { useCallback, useEffect, useMemo, useState, type RefObject } from "react";

type VirtualListOptions = {
  count: number;
  itemHeight: number;
  overscan?: number;
  containerRef: RefObject<HTMLElement | null>;
  enabled?: boolean;
};

/** Fixed-height windowing for long lists — zero external deps. */
export function useVirtualList({
  count,
  itemHeight,
  overscan = 4,
  containerRef,
  enabled = true,
}: VirtualListOptions) {
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  useEffect(() => {
    const el = containerRef.current;
    if (!el || !enabled || typeof ResizeObserver === "undefined") return;
    const measure = () => setViewportHeight(el.clientHeight);
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    measure();
    return () => ro.disconnect();
  }, [containerRef, enabled]);

  const onScroll = useCallback(() => {
    const el = containerRef.current;
    if (el) setScrollTop(el.scrollTop);
  }, [containerRef]);

  return useMemo(() => {
    if (!enabled || count === 0) {
      return { start: 0, end: count, totalHeight: 0, offsetY: 0, onScroll };
    }
    const totalHeight = count * itemHeight;
    const start = Math.max(0, Math.floor(scrollTop / itemHeight) - overscan);
    const visible = Math.ceil(Math.max(viewportHeight, itemHeight) / itemHeight) + overscan * 2;
    const end = Math.min(count, start + visible);
    return { start, end, totalHeight, offsetY: start * itemHeight, onScroll };
  }, [count, itemHeight, enabled, onScroll, overscan, scrollTop, viewportHeight]);
}

type VirtualGridOptions = {
  count: number;
  columns: number;
  rowHeight: number;
  overscan?: number;
  containerRef: RefObject<HTMLElement | null>;
  enabled?: boolean;
};

/** Row-based grid windowing; each row holds `columns` items. */
export function useVirtualGrid({
  count,
  columns,
  rowHeight,
  overscan = 2,
  containerRef,
  enabled = true,
}: VirtualGridOptions) {
  const [scrollTop, setScrollTop] = useState(0);
  const [viewportHeight, setViewportHeight] = useState(0);

  useEffect(() => {
    const el = containerRef.current;
    if (!el || !enabled || typeof ResizeObserver === "undefined") return;
    const measure = () => setViewportHeight(el.clientHeight);
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    measure();
    return () => ro.disconnect();
  }, [containerRef, enabled]);

  const onScroll = useCallback(() => {
    const el = containerRef.current;
    if (el) setScrollTop(el.scrollTop);
  }, [containerRef]);

  return useMemo(() => {
    const rowCount = Math.max(1, Math.ceil(count / Math.max(1, columns)));
    if (!enabled || count === 0) {
      return {
        startRow: 0,
        endRow: rowCount,
        startIndex: 0,
        endIndex: count,
        totalHeight: 0,
        offsetY: 0,
        onScroll,
      };
    }
    const totalHeight = rowCount * rowHeight;
    const startRow = Math.max(0, Math.floor(scrollTop / rowHeight) - overscan);
    const visibleRows = Math.ceil(Math.max(viewportHeight, rowHeight) / rowHeight) + overscan * 2;
    const endRow = Math.min(rowCount, startRow + visibleRows);
    const startIndex = startRow * columns;
    const endIndex = Math.min(count, endRow * columns);
    return {
      startRow,
      endRow,
      startIndex,
      endIndex,
      totalHeight,
      offsetY: startRow * rowHeight,
      onScroll,
    };
  }, [columns, count, enabled, onScroll, overscan, rowHeight, scrollTop, viewportHeight]);
}

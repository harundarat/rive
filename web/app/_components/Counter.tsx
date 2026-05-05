"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";

type CounterProps = {
  to: number;
  from?: number;
  duration?: number;
  prefix?: ReactNode;
  suffix?: ReactNode;
};

const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3);

export default function Counter({
  to,
  from = 0,
  duration = 1000,
  prefix,
  suffix,
}: CounterProps) {
  const ref = useRef<HTMLSpanElement | null>(null);
  const [value, setValue] = useState<number>(to);
  const startedRef = useRef(false);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      setValue(to);
      return;
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting && !startedRef.current) {
            startedRef.current = true;
            observer.disconnect();
            const start = performance.now();
            let raf = 0;
            const tick = (now: number) => {
              const t = Math.min(1, (now - start) / duration);
              const eased = easeOutCubic(t);
              setValue(from + (to - from) * eased);
              if (t < 1) {
                raf = requestAnimationFrame(tick);
              }
            };
            setValue(from);
            raf = requestAnimationFrame(tick);
            return () => cancelAnimationFrame(raf);
          }
        }
      },
      { threshold: 0.4, rootMargin: "0px 0px -10% 0px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [from, to, duration]);

  return (
    <span ref={ref}>
      {prefix}
      {Math.round(value)}
      {suffix}
    </span>
  );
}

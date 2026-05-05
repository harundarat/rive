"use client";

import { useEffect, useRef, useState } from "react";

const beforeLines = [
  { from: "scribe-04", to: "forge-12", amount: "0.42 rUSD" },
  { from: "forge-12", to: "oracle-01", amount: "0.18 rUSD" },
  { from: "scribe-04", to: "oracle-01", amount: "0.07 rUSD" },
];

const afterLines = [
  { dot: "g", text: "scribe-04 ← 1.82", side: "net cr" },
  { dot: "r", text: "forge-12 → 1.55", side: "net dr" },
  { dot: "b", text: "oracle-01 ← 0.27", side: "net cr" },
];

export default function NettingDemo() {
  const ref = useRef<HTMLDivElement | null>(null);
  const [stage, setStage] = useState(0);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      setStage(99);
      return;
    }

    const timers: number[] = [];
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            observer.disconnect();
            // Stages: 1,2,3 = each before line; 4 = divider; 5,6,7 = after lines
            const schedule = [200, 450, 700, 950, 1150, 1350, 1550];
            schedule.forEach((delay, i) => {
              timers.push(
                window.setTimeout(() => setStage(i + 1), delay),
              );
            });
            break;
          }
        }
      },
      { threshold: 0.3, rootMargin: "0px 0px -8% 0px" },
    );
    observer.observe(node);

    return () => {
      observer.disconnect();
      timers.forEach((t) => window.clearTimeout(t));
    };
  }, []);

  const showBefore = (i: number) => stage >= i + 1 || stage === 99;
  const showDivider = stage >= 4 || stage === 99;
  const showAfter = (i: number) => stage >= i + 5 || stage === 99;

  return (
    <div className="net" ref={ref}>
      <div className="net-before">
        {beforeLines.map((line, i) => (
          <div
            key={i}
            className={`net-line x${showBefore(i) ? " show" : ""}`}
          >
            <span className="a">
              {line.from} <span className="arr">→</span> {line.to}
            </span>
            <span>{line.amount}</span>
          </div>
        ))}
        <div
          className={`net-line x${showBefore(2) ? " show" : ""}`}
          style={{ opacity: showBefore(2) ? 0.25 : 0 }}
        >
          <span>+ 17 more …</span>
          <span></span>
        </div>
      </div>
      <div className={`net-divider${showDivider ? " show" : ""}`}>Net</div>
      <div className="net-after">
        {afterLines.map((line, i) => (
          <div
            key={i}
            className={`net-line summary${showAfter(i) ? " show" : ""}`}
          >
            <span className="a">
              <span className="chip">
                <span className={`d ${line.dot}`}></span>SETTLE
              </span>{" "}
              {line.text}
            </span>
            <span>{line.side}</span>
          </div>
        ))}
      </div>
    </div>
  );
}

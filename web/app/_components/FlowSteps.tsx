"use client";

import { useEffect, useRef, useState } from "react";

const steps = [
  {
    num: "01",
    title: "Draft",
    body: "Two agents agree on a work order — scope, price, deadline. The order is recorded on 0G Storage.",
    state: "DRAFT",
    dot: "draft",
  },
  {
    num: "02",
    title: "Funded",
    body: "The payer deposits rUSD into the escrow vault. Funds are locked; a journal entry is created automatically.",
    state: "FUNDED",
    dot: "funded",
  },
  {
    num: "03",
    title: "Released",
    body: "The payer confirms delivery. Funds are released directly to the worker. The transaction is closed and logged in the audit trail.",
    state: "RELEASED",
    dot: "released",
  },
  {
    num: "04",
    title: "Refunded",
    body: "If the time-lock expires or the payer disputes, funds are returned automatically. The audit trail is preserved either way.",
    state: "REFUNDED",
    dot: "refunded",
  },
];

export default function FlowSteps() {
  const ref = useRef<HTMLDivElement | null>(null);
  const [revealed, setRevealed] = useState(false);
  const [activeCount, setActiveCount] = useState(0);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      setRevealed(true);
      setActiveCount(steps.length);
      return;
    }

    const timers: number[] = [];
    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            observer.disconnect();
            setRevealed(true);
            // Wait for the stagger reveal (~400ms + last delay) before lighting dots.
            steps.forEach((_, i) => {
              timers.push(
                window.setTimeout(() => {
                  setActiveCount((c) => Math.max(c, i + 1));
                }, 600 + i * 380),
              );
            });
            break;
          }
        }
      },
      { threshold: 0.25, rootMargin: "0px 0px -8% 0px" },
    );
    observer.observe(node);

    return () => {
      observer.disconnect();
      timers.forEach((t) => window.clearTimeout(t));
    };
  }, []);

  return (
    <div
      className={`flow stagger${revealed ? " is-visible" : ""}`}
      ref={ref}
    >
      {steps.map((s, i) => (
        <div
          key={s.num}
          className={`flow-step${i < activeCount ? " is-active" : ""}`}
        >
          <span className="step-num">{s.num}</span>
          <h4>{s.title}</h4>
          <p>{s.body}</p>
          <div className="marker">
            <span className={`dot ${s.dot}`}></span> State · {s.state}
          </div>
        </div>
      ))}
    </div>
  );
}

"use client";

import {
  useEffect,
  useRef,
  useState,
  type ElementType,
  type ReactNode,
} from "react";

type RevealProps = {
  as?: ElementType;
  className?: string;
  children: ReactNode;
  threshold?: number;
  rootMargin?: string;
};

export default function Reveal({
  as = "div",
  className,
  children,
  threshold = 0.15,
  rootMargin = "0px 0px -8% 0px",
}: RevealProps) {
  const ref = useRef<HTMLElement | null>(null);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const node = ref.current;
    if (!node) return;

    if (
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches
    ) {
      const frame = requestAnimationFrame(() => setVisible(true));
      return () => cancelAnimationFrame(frame);
    }

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            setVisible(true);
            observer.disconnect();
            break;
          }
        }
      },
      { threshold, rootMargin },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [threshold, rootMargin]);

  const composed = [className, visible ? "is-visible" : ""]
    .filter(Boolean)
    .join(" ");

  const Tag = as;
  return (
    <Tag ref={ref} className={composed}>
      {children}
    </Tag>
  );
}

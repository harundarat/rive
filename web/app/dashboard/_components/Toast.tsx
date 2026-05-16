"use client";

import { useState, useEffect } from "react";

export function Toast() {
  const [msg, setMsg] = useState<string | null>(null);

  useEffect(() => {
    function handler(e: Event) {
      const detail = (e as CustomEvent<string>).detail;
      setMsg(detail);
      setTimeout(() => setMsg(null), 1400);
    }
    window.addEventListener("rive-toast", handler);
    return () => window.removeEventListener("rive-toast", handler);
  }, []);

  if (!msg) return null;
  return <div className="db-toast">{msg}</div>;
}

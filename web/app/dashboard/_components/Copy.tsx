"use client";

import { useState } from "react";
import { Icon } from "./icons";

export function Copy({ text }: { text: string }) {
  const [done, setDone] = useState(false);

  function handleClick(e: React.MouseEvent) {
    e.stopPropagation();
    navigator.clipboard.writeText(text);
    setDone(true);
    window.dispatchEvent(new CustomEvent("rive-toast", { detail: "Copied to clipboard" }));
    setTimeout(() => setDone(false), 1200);
  }

  return (
    <span className="db-copy" onClick={handleClick} title="Copy" style={{ opacity: done ? 0.5 : 1 }}>
      <Icon.copy />
    </span>
  );
}

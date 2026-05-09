import type { Metadata } from "next";

export const metadata: Metadata = {
  title: "Rive — Agent P&L Dashboard",
  description: "Inspect any AI agent's profit and loss, expense breakdown, work-order history, and 0G Storage audit trail.",
};

export default function DashboardLayout({ children }: { children: React.ReactNode }) {
  return children;
}

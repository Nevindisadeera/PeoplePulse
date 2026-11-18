"use client";

import { Bell } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { api } from "@/lib/api";
import type { NotificationList } from "@/lib/types";
import { useApi } from "@/lib/use-api";
import { Empty, IconButton, Skeleton } from "./ui";

function timeAgo(iso: string): string {
  const minutes = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  return `${Math.round(hours / 24)} d ago`;
}

// NotificationsMenu is the bell in the header: an unread badge and a panel
// with the latest notifications.
export function NotificationsMenu() {
  const [open, setOpen] = useState(false);
  const [version, setVersion] = useState(0); // bumped to reload after marking as read
  const list = useApi<NotificationList>(`/notifications?pageSize=8&v=${version}`);
  const root = useRef<HTMLDivElement>(null);
  const unread = list.data?.unreadCount ?? 0;

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent ? event.key === "Escape" : !root.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);

  async function markAllRead() {
    await api("/notifications/read-all", { method: "POST" });
    setVersion((v) => v + 1);
  }

  return (
    <div ref={root} className="relative">
      <IconButton label={unread ? `Notifications, ${unread} unread` : "Notifications"} aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <Bell size={17} strokeWidth={2.2} aria-hidden="true" />
        {unread > 0 && (
          <span className="absolute -right-1 -top-1 flex h-[18px] min-w-[18px] items-center justify-center rounded-full bg-orange px-1 text-[10px] font-bold text-ground">
            {unread > 99 ? "99+" : unread}
          </span>
        )}
      </IconButton>
      {open && (
        <div className="absolute right-0 top-12 z-20 flex w-[min(22rem,calc(100vw-2rem))] flex-col gap-3 rounded-3xl bg-surface p-4 shadow-2xl ring-1 ring-line">
          <div className="flex items-center justify-between gap-3">
            <h2 className="display text-sm font-semibold">Notifications</h2>
            {unread > 0 && (
              <button type="button" onClick={markAllRead} className="rounded-full px-2 py-1 text-[11px] font-semibold text-lime hover:underline">
                Mark all as read
              </button>
            )}
          </div>
          {list.loading && <Skeleton className="h-24" />}
          {list.error && <Empty>Notifications could not be loaded.</Empty>}
          {list.data && list.data.data.length === 0 && <Empty>You have no notifications yet.</Empty>}
          {list.data && list.data.data.length > 0 && (
            <ul className="flex max-h-80 flex-col gap-2 overflow-y-auto">
              {list.data.data.map((n) => (
                <li key={n.id} className="flex gap-3 rounded-2xl bg-raised px-3 py-2.5">
                  <span className={`mt-1.5 size-2 shrink-0 rounded-full ${n.readAt ? "bg-line" : "bg-lime"}`} aria-hidden="true" />
                  <div className="min-w-0">
                    <p className="text-[13px] font-semibold leading-snug">{n.title}</p>
                    <p className="text-xs leading-snug text-muted">{n.body}</p>
                    <p className="mt-1 text-[10px] text-muted">{timeAgo(n.createdAt)}</p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}

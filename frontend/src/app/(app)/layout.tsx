"use client";

import { Building2, LayoutDashboard, LogOut, Users } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect } from "react";
import { NotificationsMenu } from "@/components/notifications-menu";
import { IconButton, Logo, Skeleton } from "@/components/ui";
import { useAuth } from "@/lib/auth";
import { initials } from "@/lib/format";

// Sections are added here as their pages are built. A section is shown when
// the role holds any of its permissions; the dashboard is for everyone.
const NAV = [
  { href: "/", label: "Dashboard", icon: LayoutDashboard, permissions: [] as string[] },
  { href: "/employees", label: "Employees", icon: Users, permissions: ["employee.read", "employee.read_department"] },
  { href: "/departments", label: "Departments", icon: Building2, permissions: ["department.read"] },
];

export default function AppLayout({ children }: { children: React.ReactNode }) {
  const auth = useAuth();
  const router = useRouter();
  const pathname = usePathname();

  useEffect(() => {
    if (auth.status === "signedOut") router.replace("/login");
  }, [auth.status, router]);

  if (auth.status !== "signedIn") {
    return (
      <div className="mx-auto flex max-w-[1400px] flex-col gap-6 p-6" aria-busy="true" aria-label="Loading">
        <Skeleton className="h-12 w-72 rounded-full" />
        <Skeleton className="h-[70vh] rounded-card" />
      </div>
    );
  }

  return (
    <div className="mx-auto flex min-h-dvh max-w-[1400px] flex-col gap-8 px-4 py-5 sm:px-6 lg:px-8">
      <header className="flex flex-wrap items-center gap-3">
        <Link href="/" aria-label="PeoplePulse home" className="rounded-full">
          <Logo />
        </Link>
        <nav aria-label="Main" className="flex flex-wrap items-center gap-2">
          {NAV.filter((item) => item.permissions.length === 0 || item.permissions.some(auth.can)).map(({ href, label, icon: Icon }) => {
            const active = href === "/" ? pathname === "/" : pathname.startsWith(href);
            return (
              <Link
                key={href}
                href={href}
                aria-current={active ? "page" : undefined}
                className={`inline-flex h-10 items-center gap-2 rounded-full px-4 text-[13px] font-semibold transition-colors ${active ? "bg-lime text-ground" : "bg-raised text-ink hover:bg-line"}`}
              >
                <Icon size={16} strokeWidth={2.2} aria-hidden="true" />
                {label}
              </Link>
            );
          })}
        </nav>
        <div className="ml-auto flex items-center gap-3">
          <NotificationsMenu />
          <div className="hidden text-right leading-tight sm:block">
            <div className="text-[13px] font-semibold">{auth.user.fullName}</div>
            <div className="text-[11px] text-muted">{auth.role.name}</div>
          </div>
          <span className="flex size-10 items-center justify-center rounded-full bg-raised text-xs font-bold" aria-hidden="true">
            {initials(auth.user.fullName)}
          </span>
          <IconButton label="Sign out" onClick={() => auth.signOut()}>
            <LogOut size={16} strokeWidth={2.2} aria-hidden="true" />
          </IconButton>
        </div>
      </header>
      <main className="flex flex-1 flex-col gap-6 pb-8">{children}</main>
    </div>
  );
}

import {
  Bell,
  BookOpen,
  Building2,
  ChartColumn,
  FileText,
  Globe,
  KeySquare,
  LayoutDashboard,
  ScrollText,
  Settings,
  Radio,
  Webhook,
  ListTodo,
  Ticket,
  UserCog,
  Users,
  Workflow,
} from "lucide-react";
export interface NavItem {
  to: string;
  title: string;
  icon: typeof LayoutDashboard;
  description: string;
  /** Catalog permission gating the item; hidden when the caller lacks it. */
  permission?: [string, string];
  /** i18n key; falls back to the English title. */
  key?: import("@/shared/lib/i18n").TKey;
}

export const NAV_MAIN: NavItem[] = [
  {
    to: "/admin",
    key: "nav.dashboard",
    title: "Dashboard",
    icon: LayoutDashboard,
    description: "System counters, charts, live activity",
  },
  {
    to: "/admin/users",
    title: "Users",
    key: "nav.users",
    icon: Users,
    description: "Accounts, search, CRUD",
  },
  {
    to: "/admin/jobs",
    title: "Jobs",
    key: "nav.jobs",
    icon: ListTodo,
    description: "Background queue, live status",
  },
  {
    to: "/admin/enrichment",
    title: "Enrichment",
    icon: Workflow,
    description: "External profile fetching",
  },
  {
    to: "/admin/reports",
    title: "Reports",
    key: "nav.reports",
    icon: FileText,
    description: "Analytics snapshots",
  },
  {
    to: "/admin/ticket-types",
    title: "Ticket Types",
    key: "nav.ticketTypes",
    icon: Ticket,
    description: "Approval chains behind tickets",
    permission: ["ticket_types", "read"],
  },
  {
    to: "/admin/analytics",
    title: "Analytics",
    icon: ChartColumn,
    description: "Event warehouse, live trail",
  },
];

export const NAV_ADMIN: NavItem[] = [
  {
    to: "/admin/roles",
    title: "Roles",
    icon: KeySquare,
    description: "Roles and the permission matrix",
    permission: ["roles", "read"],
  },
  {
    to: "/admin/policies",
    title: "Policies",
    icon: FileText,
    description: "Raw policy rules (advanced)",
    permission: ["policies", "read"],
  },
  {
    to: "/admin/orgs",
    title: "Organizations",
    icon: Building2,
    description: "Tenants and their members",
    permission: ["orgs", "read"],
  },
  {
    to: "/admin/audit",
    title: "Audit Log",
    icon: ScrollText,
    description: "Who changed what, when",
    permission: ["audit", "read"],
  },
  {
    to: "/admin/streams",
    title: "Streams",
    key: "nav.streams",
    icon: Radio,
    description: "Outbox and dead letters",
    permission: ["streams", "read"],
  },
  {
    to: "/admin/settings",
    title: "Settings",
    icon: Settings,
    description: "Flags and runtime configuration",
    permission: ["settings", "read"],
  },
  {
    to: "/admin/webhooks",
    title: "Webhooks",
    icon: Webhook,
    description: "Outgoing event subscriptions",
    permission: ["webhooks", "read"],
  },
];

export const NAV_SECONDARY: NavItem[] = [
  {
    to: "/notifications",
    title: "Notifications",
    key: "nav.notifications",
    icon: Bell,
    description: "Your in-app alerts",
  },
  {
    to: "/profile",
    title: "Profile",
    key: "nav.profile",
    icon: UserCog,
    description: "Identities, sessions, logout",
  },
  {
    to: "/admin/getting-started",
    title: "Getting Started",
    icon: BookOpen,
    description: "How this app works",
  },
  {
    to: "/dashboard",
    key: "nav.clientApp",
    title: "Client App",
    icon: Globe,
    description: "The user-facing side of the service",
  },
];

/**
 * AppShell is the authenticated layout: collapsible sidebar, header with
 * theme picker and the SSE status badge, and the ⌘K command palette. It
 * renders inside the _auth route so every authenticated page shares it.
 */

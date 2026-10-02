/**
 * The seeded demo accounts, mirroring internal/bootstrap/demodata.go: six
 * platform accounts (demo-u-0001…0006, password `password123`) plus the
 * Acme org lead. The picker renders them as a one-click table on the login
 * page and inside the "switch account" dialogs, so the whole approval flow
 * can be walked role by role.
 */

export interface DemoAccount {
  email: string;
  password: string;
  name: string;
  roles: string[];
  /** Which home the account lands on after sign-in. */
  area: "client" | "admin";
  whatToCheck: string;
  whatToCheckRu: string;
}

export const demoAccounts: DemoAccount[] = [
  {
    email: "demo-u-0004@demo.example",
    password: "password123",
    name: "Rita Requester",
    roles: ["requester"],
    area: "client",
    whatToCheck: "Client app only: files and tracks tickets; /admin redirects away",
    whatToCheckRu: "Только клиентская часть: подаёт и отслеживает заявки; в /admin не пускает",
  },
  {
    email: "demo-u-0005@demo.example",
    password: "password123",
    name: "Ari Approver",
    roles: ["approver"],
    area: "client",
    whatToCheck: "Client + Approvals inbox: decides the final step of a chain",
    whatToCheckRu: "Клиент + инбокс согласования: решает финальный шаг цепочки",
  },
  {
    email: "demo-u-0006@demo.example",
    password: "password123",
    name: "Sam Staff",
    roles: ["requester", "approver"],
    area: "client",
    whatToCheck: "Both client sides: files tickets and approves them",
    whatToCheckRu: "Обе клиентские роли: подаёт заявки и сам их согласовывает",
  },
  {
    email: "demo-u-0002@demo.example",
    password: "password123",
    name: "Max Manager",
    roles: ["manager"],
    area: "admin",
    whatToCheck: "Admin console + filled approvals inbox (first chain step)",
    whatToCheckRu: "Админка + заполненный инбокс согласования (первый шаг цепочки)",
  },
  {
    email: "demo-u-0001@demo.example",
    password: "password123",
    name: "Ada Admin",
    roles: ["admin"],
    area: "admin",
    whatToCheck: "Full admin console, org-wide tabs, decides any approval step",
    whatToCheckRu: "Полная админка, вкладка «Все (орг)», решает любой шаг",
  },
  {
    email: "demo-u-0003@demo.example",
    password: "password123",
    name: "Vera Viewer",
    roles: ["viewer"],
    area: "admin",
    whatToCheck: "Read-only admin console",
    whatToCheckRu: "Админка только для чтения",
  },
  {
    email: "demo-u-0010@demo.example",
    password: "password123",
    name: "Acme Lead",
    roles: ["manager"],
    area: "admin",
    whatToCheck: "Manager of the Acme org: sees only Acme tickets (org scope)",
    whatToCheckRu: "Менеджер организации Acme: видит заявки только своей организации",
  },
];

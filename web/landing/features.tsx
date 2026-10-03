import { useEffect, useState, type ComponentType, type SVGProps } from "react";
import type { FeatureId } from "./feature-ids";
import { useLandingI18n } from "./i18n";
import {
  ActivityIcon,
  ArrowRightIcon,
  BarChart3Icon,
  BellIcon,
  BuildingIcon,
  CheckIcon,
  CloseIcon,
  DownloadIcon,
  FilesIcon,
  HistoryIcon,
  KeyRoundIcon,
  ListChecksIcon,
  RocketIcon,
  ScrollTextIcon,
  SearchIcon,
  ShieldCheckIcon,
  SlidersIcon,
  TerminalIcon,
  UsersIcon,
  WebhookIcon,
  WorkflowIcon,
} from "./icons";

const FEATURES: readonly {
  id: FeatureId;
  icon: ComponentType<SVGProps<SVGSVGElement>>;
}[] = [
  { id: "auth", icon: ShieldCheckIcon },
  { id: "rbac", icon: UsersIcon },
  { id: "orgs", icon: BuildingIcon },
  { id: "tickets", icon: ScrollTextIcon },
  { id: "audit", icon: HistoryIcon },
  { id: "settings", icon: SlidersIcon },
  { id: "jobs", icon: ListChecksIcon },
  { id: "streams", icon: WorkflowIcon },
  { id: "webhooks", icon: WebhookIcon },
  { id: "notifications", icon: BellIcon },
  { id: "apikeys", icon: KeyRoundIcon },
  { id: "files", icon: FilesIcon },
  { id: "export", icon: DownloadIcon },
  { id: "search", icon: SearchIcon },
  { id: "resilience", icon: ActivityIcon },
  { id: "observability", icon: BarChart3Icon },
  { id: "deploy", icon: RocketIcon },
  { id: "dx", icon: TerminalIcon },
];

/**
 * The feature explorer: a compact card per feature area (icon, title,
 * one-line teaser); clicking a card opens a native <dialog> with the full
 * description and a "what's inside" list. Every dialog is prerendered into
 * the static HTML (closed), so crawlers and no-JS visitors still see all
 * the copy — showModal() only upgrades the interaction.
 */
export function FeaturesSection() {
  const { t } = useLandingI18n();
  const [openId, setOpenId] = useState<FeatureId | null>(null);

  useEffect(() => {
    if (!openId) return;
    const dialog = document.getElementById(`feature-dialog-${openId}`);
    if (dialog instanceof HTMLDialogElement) dialog.showModal();
  }, [openId]);

  return (
    <section id="features" aria-labelledby="features-title" className="border-t border-border/60">
      <div className="mx-auto max-w-6xl px-4 py-16 md:py-24">
        <div data-reveal className="max-w-2xl">
          <h2 id="features-title" className="text-3xl font-bold tracking-tight text-balance">
            {t.features.title}
          </h2>
          <p className="mt-3 text-muted-foreground">{t.features.subtitle}</p>
        </div>
        <ul className="mt-12 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {FEATURES.map(({ id, icon: Icon }) => {
            const copy = t.features.items[id];
            return (
              <li
                key={id}
                data-reveal
                className="group relative rounded-2xl border border-border bg-card p-5 transition-all duration-200 hover:-translate-y-0.5 hover:border-primary/40 hover:shadow-lg hover:shadow-primary/5"
              >
                <span className="flex size-10 items-center justify-center rounded-xl bg-linear-to-br from-sky-500/10 to-violet-600/10 transition-colors group-hover:from-sky-500/20 group-hover:to-violet-600/20">
                  <Icon className="size-5 text-primary" aria-hidden />
                </span>
                <h3 className="mt-4 font-semibold tracking-tight">
                  <button
                    type="button"
                    aria-haspopup="dialog"
                    onClick={() => setOpenId(id)}
                    className="rounded-sm text-left after:absolute after:inset-0 after:rounded-2xl after:content-[''] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
                  >
                    {copy.title}
                  </button>
                </h3>
                <p className="mt-1.5 text-sm text-muted-foreground">{copy.teaser}</p>
                <span
                  aria-hidden
                  className="mt-4 inline-flex items-center gap-1 text-sm font-medium text-primary"
                >
                  {t.features.more}
                  <ArrowRightIcon className="size-4 transition-transform duration-200 group-hover:translate-x-0.5" />
                </span>
              </li>
            );
          })}
        </ul>
      </div>

      {FEATURES.map(({ id, icon: Icon }) => {
        const copy = t.features.items[id];
        return (
          // The onClick implements backdrop-click-to-close: clicks on the
          // ::backdrop land on the <dialog> element itself, never on the
          // card content. Keyboard users have Escape (native) and the
          // close button, so the click handler needs no keyboard twin.
          // oxlint-disable-next-line click-events-have-key-events no-noninteractive-element-interactions
          <dialog
            key={id}
            id={`feature-dialog-${id}`}
            aria-labelledby={`feature-dialog-title-${id}`}
            onClose={() => setOpenId(null)}
            onClick={(event) => {
              if (event.target === event.currentTarget) event.currentTarget.close();
            }}
            className="feature-dialog"
          >
            <div className="p-6 sm:p-8">
              <div className="flex items-start justify-between gap-4">
                <span className="flex size-11 items-center justify-center rounded-xl bg-linear-to-br from-sky-500/15 to-violet-600/15">
                  <Icon className="size-5.5 text-primary" aria-hidden />
                </span>
                <button
                  type="button"
                  aria-label={t.features.dialogClose}
                  onClick={() => {
                    const dialog = document.getElementById(`feature-dialog-${id}`);
                    if (dialog instanceof HTMLDialogElement) dialog.close();
                  }}
                  className="rounded-full p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-primary"
                >
                  <CloseIcon className="size-5" aria-hidden />
                </button>
              </div>
              <h3
                id={`feature-dialog-title-${id}`}
                className="mt-4 text-xl font-semibold tracking-tight"
              >
                {copy.title}
              </h3>
              <p className="mt-3 text-sm leading-6 text-muted-foreground">{copy.description}</p>
              <p className="mt-6 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                {t.features.whatsInside}
              </p>
              <ul className="mt-3 space-y-2">
                {copy.bullets.map((bullet) => (
                  <li key={bullet} className="flex items-start gap-2.5 text-sm">
                    <CheckIcon className="mt-0.5 size-4 shrink-0 text-emerald-500" aria-hidden />
                    {bullet}
                  </li>
                ))}
              </ul>
            </div>
          </dialog>
        );
      })}
    </section>
  );
}

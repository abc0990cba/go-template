import { useEffect } from "react";

/**
 * Progressive scroll-reveal. Without JavaScript nothing is hidden: the
 * hook only arms elements (adds .reveal-armed) that sit below the fold,
 * after hydration, and only when the visitor allows motion. The observer
 * then swaps .reveal-armed for .reveal-in as each element scrolls into
 * view. The CSS for both classes lives under prefers-reduced-motion:
 * no-preference, so reduced-motion users never see a flash either.
 */
export function useRevealOnScroll() {
  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    if (!("IntersectionObserver" in window)) return;

    const elements = Array.from(document.querySelectorAll<HTMLElement>("[data-reveal]"));
    if (elements.length === 0) return;

    const observer = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (entry.isIntersecting) {
            entry.target.classList.add("reveal-in");
            observer.unobserve(entry.target);
          }
        }
      },
      // Start slightly before the element fully enters, so the motion reads
      // as a reaction to scrolling rather than lag behind it.
      { rootMargin: "0px 0px -8% 0px", threshold: 0.08 },
    );

    for (const element of elements) {
      if (element.getBoundingClientRect().top > window.innerHeight * 0.9) {
        element.classList.add("reveal-armed");
        observer.observe(element);
      }
    }
    return () => observer.disconnect();
  }, []);
}

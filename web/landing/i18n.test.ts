import { describe, expect, it } from "vitest";
import { FEATURE_IDS } from "./feature-ids";
import { LOCALES, pickLocale, type Locale } from "./i18n";
import { de } from "./locales/de";
import { en, type Dictionary } from "./locales/en";
import { fr } from "./locales/fr";
import { ru } from "./locales/ru";

/**
 * The four locales must stay in lockstep. tsc already fails the build when
 * a translation misses a Dictionary key; these tests guard the things the
 * type system cannot see — bullet counts, non-empty copy per feature — and
 * pin the browser-language resolution rules.
 */

function keyPaths(value: unknown, prefix = ""): string[] {
  if (value === null || typeof value !== "object") return [prefix];
  if (Array.isArray(value)) {
    return value.flatMap((item, index) => keyPaths(item, `${prefix}[${index}]`));
  }
  return Object.keys(value).flatMap((key) =>
    keyPaths((value as Record<string, unknown>)[key], prefix ? `${prefix}.${key}` : key),
  );
}

describe("landing locales", () => {
  const all: Record<Locale, Dictionary> = { en, ru, fr, de };

  it("have identical key trees across en/ru/fr/de", () => {
    const reference = keyPaths(en).toSorted();
    for (const locale of LOCALES) {
      expect(keyPaths(all[locale]).toSorted(), `locale ${locale} diverges from en`).toEqual(
        reference,
      );
    }
  });

  it("cover every feature id with non-empty copy in every locale", () => {
    for (const locale of LOCALES) {
      for (const id of FEATURE_IDS) {
        const copy = all[locale].features.items[id];
        expect(copy.title.trim(), `${locale}/${id} title`).not.toBe("");
        expect(copy.teaser.trim(), `${locale}/${id} teaser`).not.toBe("");
        expect(copy.description.trim(), `${locale}/${id} description`).not.toBe("");
        expect(copy.bullets.length, `${locale}/${id} bullets`).toBeGreaterThanOrEqual(3);
      }
    }
  });
});

describe("pickLocale", () => {
  it("prefers an explicitly stored choice", () => {
    expect(pickLocale("de", ["fr-FR", "fr"])).toBe("de");
  });

  it("ignores a stored value that is not a supported locale", () => {
    expect(pickLocale("es", ["en-US"])).toBe("en");
  });

  it("matches the first navigator language by prefix", () => {
    expect(pickLocale(null, ["fr-CA", "fr", "en-US"])).toBe("fr");
    expect(pickLocale(null, ["ru"])).toBe("ru");
    expect(pickLocale(null, ["de-AT", "de"])).toBe("de");
  });

  it("falls back to English", () => {
    expect(pickLocale(null, ["es-ES", "it"])).toBe("en");
    expect(pickLocale(null, [])).toBe("en");
  });
});

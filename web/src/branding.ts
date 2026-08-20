import defaultFaviconURL from "@/assets/brand/pokernode-linked-cards-porcelain.svg";
import { DEFAULT_BRANDING_CONFIG, type BrandingConfig } from "@/types";

export { defaultFaviconURL as DEFAULT_FAVICON_URL };

export function normalizeBranding(config?: Partial<BrandingConfig> | null): BrandingConfig {
  return { ...DEFAULT_BRANDING_CONFIG, ...(config || {}) };
}

export function applyDocumentBranding(config: BrandingConfig) {
  document.title = config.page_title;

  let applicationName = document.querySelector<HTMLMetaElement>('meta[name="application-name"]');
  if (!applicationName) {
    applicationName = document.createElement("meta");
    applicationName.name = "application-name";
    document.head.append(applicationName);
  }
  applicationName.content = config.site_name;

  let favicon = document.querySelector<HTMLLinkElement>('link[rel~="icon"]');
  if (!favicon) {
    favicon = document.createElement("link");
    favicon.rel = "icon";
    document.head.append(favicon);
  }
  favicon.removeAttribute("type");
  favicon.href = config.favicon_url || defaultFaviconURL;
}

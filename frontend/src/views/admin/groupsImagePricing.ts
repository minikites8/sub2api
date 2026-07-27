export const imagePricingPlatforms = new Set([
  "antigravity",
  "gemini",
  "grok",
  "openai",
]);

export const supportsImagePricingPlatform = (platform: string): boolean =>
  imagePricingPlatforms.has(platform);

export const videoPricingPlatforms = new Set(["grok", "baidu_vod"]);

export const supportsVideoPricingPlatform = (platform: string): boolean =>
  videoPricingPlatforms.has(platform);

export const imagePricingI18nKey = (_platform: string, key: string): string =>
  `admin.groups.imagePricing.${key}`;

export const videoPricingI18nKey = (key: string): string =>
  `admin.groups.videoPricing.${key}`;

type ImagePricingTierKey = "image_price_1k" | "image_price_2k" | "image_price_4k";
type VideoPricingTierKey =
  | "video_price_480p"
  | "video_price_720p"
  | "video_price_1080p"
  | "video_price_4k";

const defaultImagePricePlaceholders: Record<
  string,
  Record<ImagePricingTierKey, string>
> = {
  default: {
    image_price_1k: "0.134",
    image_price_2k: "0.201",
    image_price_4k: "0.268",
  },
  grok: {
    image_price_1k: "0.02",
    image_price_2k: "0.02",
    image_price_4k: "0.02",
  },
};

// 视频价为每秒单价（Credits/s）。占位值用于展示常用模型的默认价格。
const defaultVideoPricePlaceholders: Record<
  string,
  Record<VideoPricingTierKey, string>
> = {
  grok: {
    video_price_480p: "0.05",
    video_price_720p: "0.07",
    video_price_1080p: "0.25",
    video_price_4k: "",
  },
  baidu_vod: {
    video_price_480p: "",
    video_price_720p: "0.9",
    // HappyHorse 1.0 and 1.1 use model-specific 1080p defaults in the backend.
    video_price_1080p: "",
    // Seedance 2.0 uses its model-specific 4K default in the backend.
    video_price_4k: "",
  },
};

export const getImagePricePlaceholder = (
  platform: string,
  tier: ImagePricingTierKey,
): string => {
  const card = defaultImagePricePlaceholders[platform] ?? defaultImagePricePlaceholders.default;
  return card[tier];
};

export const getVideoPricePlaceholder = (
  platform: string,
  tier: VideoPricingTierKey,
): string => {
  const card = defaultVideoPricePlaceholders[platform];
  return card?.[tier] ?? "";
};

export const getDefaultImagePreviewPrice = (
  platform: string,
  tier: ImagePricingTierKey,
): number | null => {
  const placeholder = getImagePricePlaceholder(platform, tier);
  if (placeholder === "") {
    return null;
  }
  const value = Number(placeholder);
  return Number.isFinite(value) ? value : null;
};

export const getDefaultVideoPreviewPrice = (
  platform: string,
  tier: VideoPricingTierKey,
): number | null => {
  const placeholder = getVideoPricePlaceholder(platform, tier);
  if (placeholder === "") {
    return null;
  }
  const value = Number(placeholder);
  return Number.isFinite(value) ? value : null;
};

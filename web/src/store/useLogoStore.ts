import { ref, computed } from 'vue';
import { defineStore } from 'pinia';

const STORAGE_KEY = 'custom-logo';
const DEFAULT_LOGO = '/assets/logo.png';
const MAX_FILE_SIZE = 2 * 1024 * 1024; // 2 MB

export const useLogoStore = defineStore('logo', () => {
  const customLogo = ref<string | null>(localStorage.getItem(STORAGE_KEY));

  const logoSrc = computed(() => customLogo.value || DEFAULT_LOGO);
  const isCustom = computed(() => customLogo.value !== null);

  // The in-page <img> is rounded with CSS, but a browser tab favicon is
  // painted by the browser chrome straight from the image bytes — CSS can't
  // touch it. To get a round tab icon we crop the source into a circle
  // (transparent corners) on a canvas before using it as the favicon.
  const FAVICON_SIZE = 64;
  const toRoundFavicon = (src: string): Promise<string> =>
    new Promise((resolve, reject) => {
      const img = new Image();
      img.onload = () => {
        const canvas = document.createElement('canvas');
        canvas.width = FAVICON_SIZE;
        canvas.height = FAVICON_SIZE;
        const ctx = canvas.getContext('2d');
        if (!ctx) {
          reject(new Error('2D canvas context unavailable'));
          return;
        }
        ctx.beginPath();
        ctx.arc(FAVICON_SIZE / 2, FAVICON_SIZE / 2, FAVICON_SIZE / 2, 0, Math.PI * 2);
        ctx.clip();
        const scale = Math.max(FAVICON_SIZE / img.width, FAVICON_SIZE / img.height);
        const w = img.width * scale;
        const h = img.height * scale;
        ctx.drawImage(img, (FAVICON_SIZE - w) / 2, (FAVICON_SIZE - h) / 2, w, h);
        resolve(canvas.toDataURL('image/png'));
      };
      img.onerror = () => reject(new Error('Failed to load image for favicon'));
      img.src = src;
    });

  const applyFavicon = async () => {
    const href = await toRoundFavicon(logoSrc.value).catch(() => logoSrc.value);

    // Mutating an existing <link>'s href doesn't reliably repaint the tab
    // icon in Firefox/Chrome — they only pick up a fresh element.
    document
      .querySelectorAll<HTMLLinkElement>('link[rel="icon"]')
      .forEach((el) => el.remove());
    const link = document.createElement('link');
    link.rel = 'icon';
    link.href = href;
    document.head.appendChild(link);
  };

  const setLogoFromFile = (file: File): Promise<void> => {
    if (!file.type.startsWith('image/')) {
      return Promise.reject(new Error('File must be an image'));
    }
    if (file.size > MAX_FILE_SIZE) {
      return Promise.reject(new Error('Image must be smaller than 2 MB'));
    }

    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => {
        const dataUrl = reader.result as string;
        customLogo.value = dataUrl;
        localStorage.setItem(STORAGE_KEY, dataUrl);
        applyFavicon();
        resolve();
      };
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(file);
    });
  };

  const resetLogo = () => {
    customLogo.value = null;
    localStorage.removeItem(STORAGE_KEY);
    applyFavicon();
  };

  // Apply whatever logo is already stored (or the default) on store creation.
  applyFavicon();

  return { logoSrc, isCustom, setLogoFromFile, resetLogo };
});

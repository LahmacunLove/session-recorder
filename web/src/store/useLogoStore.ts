import { ref, computed } from 'vue';
import { defineStore } from 'pinia';

const STORAGE_KEY = 'custom-logo';
const DEFAULT_LOGO = '/assets/logo.png';
const MAX_FILE_SIZE = 2 * 1024 * 1024; // 2 MB

export const useLogoStore = defineStore('logo', () => {
  const customLogo = ref<string | null>(localStorage.getItem(STORAGE_KEY));

  const logoSrc = computed(() => customLogo.value || DEFAULT_LOGO);
  const isCustom = computed(() => customLogo.value !== null);

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
        resolve();
      };
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(file);
    });
  };

  const resetLogo = () => {
    customLogo.value = null;
    localStorage.removeItem(STORAGE_KEY);
  };

  return { logoSrc, isCustom, setLogoFromFile, resetLogo };
});

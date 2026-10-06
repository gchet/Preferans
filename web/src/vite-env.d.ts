/// <reference types="vite/client" />

interface Window {
  preferansExit?: () => Promise<void>;
  preferansLanguage?: (language: string) => Promise<void>;
  PreferansAndroid?: {
    exitApp(): void;
    saveCode(code: string): void;
    saveLog(log: string): void;
    saveFullLogToDownloads?(): void;
    getVersionCode(): number;
    setLanguage?(language: string): void;
    getVersionName(): string;
    checkUpdate(url: string, requestId: number): void;
    installUpdate(url: string): void;
  };
}

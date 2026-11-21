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
    setKeepAlive?(on: boolean): void;
    setVoiceMode?(on: boolean): void;
    audioInfo?(): string;
    getVersionName(): string;
    installUpdate(url: string, sha256: string): void;
  };
}

declare const __APP_VERSION__: string;

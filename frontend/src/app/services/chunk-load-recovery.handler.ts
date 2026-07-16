import { ErrorHandler, Injectable } from '@angular/core';

const RECOVERY_KEY = 'hai.chunk-load-recovery-at';
const RECOVERY_WINDOW_MS = 30_000;

@Injectable()
export class ChunkLoadRecoveryHandler implements ErrorHandler {
  handleError(error: unknown): void {
    if (this.isChunkLoadError(error) && this.canRecover()) {
      sessionStorage.setItem(RECOVERY_KEY, String(Date.now()));
      window.location.reload();
      return;
    }

    console.error(error);
  }

  private isChunkLoadError(error: unknown): boolean {
    const message = error instanceof Error ? `${error.name} ${error.message}` : String(error || '');
    return /ChunkLoadError|Loading chunk .* failed/i.test(message);
  }

  private canRecover(): boolean {
    const previousRecovery = Number(sessionStorage.getItem(RECOVERY_KEY) || '0');
    return !previousRecovery || Date.now() - previousRecovery > RECOVERY_WINDOW_MS;
  }
}

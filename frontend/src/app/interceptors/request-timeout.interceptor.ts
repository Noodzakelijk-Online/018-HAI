import { Injectable } from '@angular/core';
import { HttpEvent, HttpHandler, HttpInterceptor, HttpRequest } from '@angular/common/http';
import { Observable, timeout } from 'rxjs';

@Injectable()
export class RequestTimeoutInterceptor implements HttpInterceptor {
  static readonly readTimeoutMs = 8_000;
  static readonly operationTimeoutMs = 30_000;
  // This is the sole browser-upload path backed by the gateway's explicit
  // 750 MiB limit and 15-minute proxy timeout. Keep other mutations short.
  static readonly reviewedArchiveUploadTimeoutMs = 15 * 60_000;

  private static readonly reviewedArchiveUploadPath = '/api/v1/agent-runtimes/openclaw/ecosystem/upload';

  intercept(request: HttpRequest<unknown>, next: HttpHandler): Observable<HttpEvent<unknown>> {
    const duration = request.method === 'GET'
      ? RequestTimeoutInterceptor.readTimeoutMs
      : this.isReviewedArchiveUpload(request)
        ? RequestTimeoutInterceptor.reviewedArchiveUploadTimeoutMs
        : RequestTimeoutInterceptor.operationTimeoutMs;
    return next.handle(request).pipe(timeout(duration));
  }

  private isReviewedArchiveUpload(request: HttpRequest<unknown>): boolean {
    return request.method === 'POST'
      && request.url.includes(RequestTimeoutInterceptor.reviewedArchiveUploadPath);
  }
}

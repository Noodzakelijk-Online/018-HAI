import { HttpClient } from '@angular/common/http';
import { Injectable } from '@angular/core';
import { Observable, map } from 'rxjs';
import {
  AuthActorRole,
  IAuthSession,
  IAuthSessionPermissions,
} from '../models/auth-session.model.interface';

type AuthSessionResponse = Partial<Omit<IAuthSession, 'permissions'>> & {
  permissions?: Partial<IAuthSessionPermissions>;
};

@Injectable({ providedIn: 'root' })
export class AuthSessionService {
  private readonly apiUrl = '/api/v1/auth/session';

  constructor(private http: HttpClient) {}

  session(): Observable<IAuthSession> {
    return this.http
      .get<AuthSessionResponse>(this.apiUrl)
      .pipe(map((response) => this.normalize(response)));
  }

  private normalize(response: AuthSessionResponse): IAuthSession {
    const subject = typeof response.subject === 'string' ? response.subject.trim() : '';
    const authenticated = response.authenticated === true && subject.length > 0;
    const permissions = response.permissions ?? {};

    return {
      authenticated,
      subject,
      role: this.role(response.role),
      permissions: {
        canRead: authenticated && permissions.canRead === true,
        canOperate: authenticated && permissions.canOperate === true,
        canApprove: authenticated && permissions.canApprove === true,
        canAdminister: authenticated && permissions.canAdminister === true,
      },
    };
  }

  private role(role: unknown): AuthActorRole {
    switch (role) {
      case 'owner':
      case 'operator':
      case 'viewer':
        return role;
      default:
        return 'unknown';
    }
  }
}

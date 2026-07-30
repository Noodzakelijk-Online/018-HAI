export type AuthActorRole = 'owner' | 'operator' | 'viewer' | 'unknown';

export interface IAuthSessionPermissions {
  canRead: boolean;
  canOperate: boolean;
  canApprove: boolean;
  canAdminister: boolean;
}

export interface IAuthSession {
  authenticated: boolean;
  subject: string;
  role: AuthActorRole;
  permissions: IAuthSessionPermissions;
}

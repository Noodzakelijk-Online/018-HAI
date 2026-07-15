import {Observable} from "rxjs";
import {IUserModel} from "../models/user.model.interface";

export interface IAuthCapabilities {
  googleLoginEnabled: boolean;
  passwordRecoveryEmailEnabled: boolean;
}

export interface IAuthService {
	getCapabilities(): Observable<IAuthCapabilities>;
  login(username: string, password: string): Observable<Object>;
  register(email: string, password: string): Observable<IUserModel>;
  requestPasswordReset(email: string): Observable<void>;
  confirmPasswordReset(token: string, newPassword: string): Observable<void>;
  logout(): Observable<void>;
  loggedIn(): Observable<boolean>;
}

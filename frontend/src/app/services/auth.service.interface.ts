import {Observable} from "rxjs";
import {IUserModel} from "../models/user.model.interface";

export interface IAuthService {
  login(username: string, password: string): Observable<Object>;
  register(email: string, password: string): Observable<IUserModel>;
  logout(): Observable<void>;
  loggedIn(): Observable<boolean>;
}

import { Injectable } from '@angular/core';
import { HttpClient } from '@angular/common/http';
import { Observable } from 'rxjs';
import { IHAIOSService } from '../hai-os.service.interface';
import { IHAIOSOverview } from '../../models/hai-os.model.interface';

@Injectable({
  providedIn: 'root',
})
export class HAIOSService implements IHAIOSService {
  private apiUrl = '/api/v1/os';

  constructor(private http: HttpClient) {}

  overview(): Observable<IHAIOSOverview> {
    return this.http.get<IHAIOSOverview>(`${this.apiUrl}/overview`);
  }
}

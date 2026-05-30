import { Observable } from 'rxjs';
import { IHAIOSOverview } from '../models/hai-os.model.interface';

export interface IHAIOSService {
  overview(): Observable<IHAIOSOverview>;
}

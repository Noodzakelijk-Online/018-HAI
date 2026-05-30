import { Observable } from 'rxjs';
import {
  IAnswerRequest,
  IVerificationResult,
  IVerificationRun,
} from '../models/verification.model.interface';

export interface IVerificationService {
  answer(request: IAnswerRequest): Observable<IVerificationResult>;
  runs(): Observable<IVerificationRun[]>;
  runDetails(id: string): Observable<IVerificationResult>;
}

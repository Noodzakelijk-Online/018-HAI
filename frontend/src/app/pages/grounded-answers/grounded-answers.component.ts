import { Component, Inject, OnInit } from '@angular/core';
import { FormBuilder, FormGroup, Validators } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { NzNotificationService } from 'ng-zorro-antd/notification';
import {
  IVerificationResult,
  IVerificationRun,
} from '../../models/verification.model.interface';
import { IResearchResult, IResearchStatus } from '../../models/research.model.interface';
import { ResearchService } from '../../services/research.service';
import { VERIFICATION_SERVICE_TOKEN } from '../../services/verification/verification.service.token';
import { IVerificationService } from '../../services/verification.service.interface';

@Component({
  selector: 'app-grounded-answers',
  templateUrl: './grounded-answers.component.html',
  styleUrls: ['./grounded-answers.component.scss'],
})
export class GroundedAnswersComponent implements OnInit {
  result?: IVerificationResult;
  runs: IVerificationRun[] = [];
  loading = false;
  researchLoading = false;
  researchStatus?: IResearchStatus;
  researchResults: IResearchResult[] = [];
  selectedResearchCandidate?: IResearchResult;

  answerForm: FormGroup = this.fb.group({
    question: ['What did connected sources say about source-grounded task context?', [Validators.required]],
    projectKey: ['018-HAI'],
    pursuitId: [''],
    mode: ['grounded'],
    includeSensitive: [false],
    allowMemoryUpdate: [false],
    evidenceLabel: ['Manual evidence'],
    evidenceUri: ['local://manual-evidence'],
    evidenceSnippet: ['Connected-source records should be checked before task planning, and unsupported claims should be marked for review.'],
    official: [false],
    primary: [true],
  });

  constructor(
    private fb: FormBuilder,
    @Inject(VERIFICATION_SERVICE_TOKEN)
    private verificationService: IVerificationService,
    private researchService: ResearchService,
    private notification: NzNotificationService,
    private route: ActivatedRoute,
    private router: Router
  ) {}

  ngOnInit(): void {
    const params = this.route.snapshot.queryParamMap;
    this.answerForm.patchValue({
      pursuitId: params.get('pursuitId') || '',
      projectKey: params.get('projectKey') || this.answerForm.value.projectKey,
      question: params.get('question') || this.answerForm.value.question,
    });
    this.loadRuns();
    this.loadResearchStatus();
  }

  answer(): void {
    if (this.answerForm.invalid) {
      return;
    }
    this.loading = true;
    const snippet = String(this.answerForm.value.evidenceSnippet || '').trim();
    this.verificationService
      .answer({
        question: this.answerForm.value.question,
        projectKey: this.answerForm.value.projectKey,
        pursuitId: this.answerForm.value.pursuitId,
        mode: this.answerForm.value.mode,
        includeSensitive: this.answerForm.value.includeSensitive,
        allowMemoryUpdate: this.answerForm.value.allowMemoryUpdate,
        externalEvidence: snippet
          ? [
              {
                sourceType: this.selectedResearchCandidate ? 'local_research' : 'manual',
                sourceLabel: this.answerForm.value.evidenceLabel,
                sourceUri: this.answerForm.value.evidenceUri,
                snippet,
                authority: this.answerForm.value.official ? 'official' : 'operator_supplied',
                freshness: 'operator_supplied',
                official: this.answerForm.value.official,
                primary: this.answerForm.value.primary,
              },
            ]
          : [],
      })
      .subscribe({
        next: (result) => {
          this.result = result;
          this.loading = false;
          if (result.pursuitLinkError) {
            this.notification.warning('Verification saved, pursuit link failed', result.pursuitLinkError);
          } else if (result.pursuitLinked) {
            this.notification.success('Verification linked', 'The verification run is now visible in the selected pursuit evidence timeline.');
          }
          this.loadRuns();
        },
        error: () => {
          this.loading = false;
          this.notification.error('Error', 'Failed to create grounded answer.');
        },
      });
  }

  loadRuns(): void {
    this.verificationService.runs().subscribe({
      next: (runs) => (this.runs = runs),
      error: () => (this.runs = []),
    });
  }

  loadResearchStatus(): void {
    this.researchService.status().subscribe({
      next: (status) => (this.researchStatus = status),
      error: () => (this.researchStatus = undefined),
    });
  }

  searchResearch(): void {
    const query = String(this.answerForm.value.question || '').trim();
    if (!query || this.researchLoading) return;
    this.researchLoading = true;
    this.researchResults = [];
    this.researchService.search(query).subscribe({
      next: (response) => {
        this.researchLoading = false;
        this.researchResults = response.results || [];
      },
      error: () => {
        this.researchLoading = false;
        this.notification.warning('Local research unavailable', 'Configure a reviewed local SearXNG instance to discover public source candidates. No evidence was added.');
        this.loadResearchStatus();
      },
    });
  }

  useResearchResult(result: IResearchResult): void {
    this.selectedResearchCandidate = result;
    this.answerForm.patchValue({
      evidenceLabel: result.title || 'Local research candidate',
      evidenceUri: result.sourceUri,
      evidenceSnippet: result.snippet,
      official: false,
      primary: false,
    });
    this.notification.info('Candidate selected', 'The source is attached as unverified evidence. Claim verification remains required.');
  }

  loadDetails(run: IVerificationRun): void {
    this.verificationService.runDetails(run.id).subscribe({
      next: (details) => {
        details.run = run;
        this.result = details;
      },
      error: () => this.notification.error('Error', 'Failed to load verification run.'),
    });
  }

  goHome(): void {
    this.router.navigate(['/home']);
  }

  clearPursuitLink(): void {
    this.answerForm.patchValue({ pursuitId: '' });
  }

  openPursuit(id?: string): void {
    const pursuitId = id || this.answerForm.value.pursuitId;
    if (pursuitId) {
      this.router.navigate(['/pursuits'], { queryParams: { selected: pursuitId } });
    }
  }
}

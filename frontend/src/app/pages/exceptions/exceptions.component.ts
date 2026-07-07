import { Component } from '@angular/core';

export interface WorkflowItem {
  id: string;
  title: string;
  state: string;
  updatedAt?: string;
}

/** States that require a human to look — everything else is "handled". */
const ATTENTION_STATES = [
  'awaiting_approval',
  'failed',
  'dead_letter',
  'blocked',
  'interrupted',
];

@Component({
  selector: 'app-exceptions',
  templateUrl: './exceptions.component.html',
  styleUrls: ['./exceptions.component.scss'],
})
export class ExceptionsComponent {
  items: WorkflowItem[] = [];

  /** Only the items that need attention, newest first when timestamps exist. */
  get exceptions(): WorkflowItem[] {
    return this.items
      .filter((item) => ExceptionsComponent.needsAttention(item.state))
      .sort((a, b) => (b.updatedAt ?? '').localeCompare(a.updatedAt ?? ''));
  }

  static needsAttention(state: string): boolean {
    return ATTENTION_STATES.includes((state ?? '').toLowerCase());
  }

  /** Replace the current items (wired to the workflow API by the caller). */
  setItems(items: WorkflowItem[]): void {
    this.items = items ?? [];
  }

  tagColor(state: string): string {
    switch ((state ?? '').toLowerCase()) {
      case 'failed':
      case 'dead_letter':
        return 'red';
      case 'blocked':
      case 'interrupted':
        return 'orange';
      case 'awaiting_approval':
        return 'gold';
      default:
        return 'default';
    }
  }
}

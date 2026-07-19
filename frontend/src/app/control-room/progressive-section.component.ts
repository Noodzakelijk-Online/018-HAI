import { Component, EventEmitter, Input, OnChanges, Output, SimpleChanges } from '@angular/core'
import { ModuleViewPreferencesService } from './module-view-preferences.service'

@Component({
  selector: 'hai-progressive-section',
  templateUrl: './progressive-section.component.html',
})
export class HaiProgressiveSectionComponent implements OnChanges {
  @Input() moduleId = ''
  @Input() sectionId = ''
  @Input() title = ''
  @Input() summary = ''
  @Input() advancedOnly = true
  @Output() openChange = new EventEmitter<boolean>()
  open = false

  constructor(private preferences: ModuleViewPreferencesService) {}

  ngOnChanges(_: SimpleChanges): void {
    if (this.moduleId && this.sectionId) this.open = this.preferences.get(this.moduleId).openSections[this.sectionId] === true
  }

  toggle(): void {
    this.open = !this.open
    this.preferences.setSection(this.moduleId, this.sectionId, this.open)
    this.openChange.emit(this.open)
  }
}

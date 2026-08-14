import { of } from 'rxjs';
import { ConnectedSourceService } from './connected-source.service';

describe('ConnectedSourceService', () => {
  it('requests a bounded source audit history when a limit is provided', (done) => {
    const http = {
      get: jasmine.createSpy('get').and.returnValue(of([])),
    };
    const service = new ConnectedSourceService(http as any);

    service.auditLogs(undefined, 8).subscribe(() => {
      const [url, options] = http.get.calls.mostRecent().args;
      expect(url).toBe('/api/v1/sources/audit-logs');
      expect(options.params.get('sourceId')).toBeNull();
      expect(options.params.get('limit')).toBe('8');
      done();
    });
  });
});

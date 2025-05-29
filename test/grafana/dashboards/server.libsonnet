local g = import 'g.libsonnet';

local p = import 'panels.libsonnet';
local queries = import 'queries.libsonnet';
local v = import 'variables.libsonnet';

local row = g.panel.row;
local q = queries.prometheus;

g.dashboard.new('Acmetel Sever')
+ g.dashboard.graphTooltip.withSharedCrosshair()
+ g.dashboard.withVariables(
  v.datasource.prometheus
)
+ g.dashboard.withPanels(
  g.util.grid.wrapPanels(
    [
      p.stat.byteRate('Received Bytes Rate', q.rate('received_bytes_total')),
      p.stat.base('Received Bytes Total', q.counter('received_bytes_total'), unit='decbytes', color='yellow', w=3),
      p.stat.base('Received Messages', q.counter('worker_pool_received_messages_total'), color='purple', w=3),
      p.stat.base(
        'Handled Messages',
        [
          q.filteredCounter('worker_pool_handled_messages_total', 'acmetel_stage_name', 'cannelloni'),
          q.filteredCounter('worker_pool_handled_messages_total', 'acmetel_stage_name', 'can'),
        ]
      ),
      p.stat.base('Delivered Messages', q.counter('worker_pool_delivered_messages_total'), color='blue', w=3),
      p.stat.base('Inserted Rows', q.counter('received_bytes_total'), color='purple', w=3),
      p.timeSeries.latency(
        'Message Processing Time',
        [
          q.quantile('total_message_processing_time', '0.95', 'p95'),
          q.quantile('total_message_processing_time', '0.9', 'p90'),
          q.quantile('total_message_processing_time', '0.75', 'p75'),
          q.quantile('total_message_processing_time', '0.5', 'p50'),
        ]
      ),
      p.timeSeries.step(
        'Active Workers',
        q.counter('worker_pool_active_workers', '{{acmetel_stage_kind}} - {{acmetel_stage_name}}')
      ),
    ],
  ),
)
+ g.dashboard.withRefresh('auto')
+ g.dashboard.time.withFrom('now-15m')
+ g.dashboard.withTimezone('browser')

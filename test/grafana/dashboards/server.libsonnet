local g = import 'g.libsonnet';

local p = import 'panels.libsonnet';
local q = import 'queries.libsonnet';
local v = import 'variables.libsonnet';

local row = g.panel.row;

local prometheus = q.prometheus;
local tempo = q.tempo;

g.dashboard.new('Acmetel Sever')
+ g.dashboard.graphTooltip.withSharedCrosshair()
+ g.dashboard.withVariables(
  [
    v.datasource.prometheus,
    v.datasource.tempo,
  ]
)
+ g.dashboard.withPanels(
  g.util.grid.wrapPanels(
    [
      p.stat.byteRate('Received Bytes Rate', prometheus.rate('received_bytes_total')),

      p.stat.base('Received Bytes Total', prometheus.counter('received_bytes_total'), unit='decbytes', color='yellow', w=3),

      p.stat.base('Received Messages', prometheus.counter('worker_pool_received_messages_total'), color='purple', w=3),

      p.stat.base(
        'Handled Messages',
        [
          prometheus.filteredCounter('worker_pool_handled_messages_total', 'acmetel_stage_name', 'cannelloni'),
          prometheus.filteredCounter('worker_pool_handled_messages_total', 'acmetel_stage_name', 'can'),
        ]
      ),

      p.stat.base('Delivered Messages', prometheus.counter('worker_pool_delivered_messages_total'), color='blue', w=3),

      p.stat.base('Inserted Rows', prometheus.counter('delivered_rows_total'), color='purple', w=3),

      p.timeSeries.latency(
        'Message Processing Time',
        [
          prometheus.quantile('total_message_processing_time', 95),
          prometheus.quantile('total_message_processing_time', 90),
          prometheus.quantile('total_message_processing_time', 75),
          prometheus.quantile('total_message_processing_time', 50),
        ]
      ),

      p.timeSeries.step(
        'Active Workers',
        prometheus.counter('worker_pool_active_workers', '{{acmetel_stage_kind}} - {{acmetel_stage_name}}')
      ),

      p.table.base('Traces', tempo.duration('sc-test-telemetry', '100ms')),
    ],
  ),
)
+ g.dashboard.time.withFrom('now-15m')
+ g.dashboard.withTimezone('browser')
+ g.dashboard.timepicker.withRefreshIntervals(['250ms', '500ms', '1s', '2s', '5s', '10s', '30s', '1m', '5m', '15m', '30m', '1h', '2h', '1d'])
+ g.dashboard.withRefresh('1s')

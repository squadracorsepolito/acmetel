local g = import 'g.libsonnet';
local var = import 'variables.libsonnet';
local prometheus = g.query.prometheus;

{
  prometheus: {
    base(expr):
      prometheus.new('$' + var.datasource.prometheus.name, expr)
      + prometheus.withInterval('15s'),

    counter(field, legend=''):
      self.base(field) + (
        if legend != '' then prometheus.withLegendFormat(legend) else {}
      ),

    filteredCounter(field, label, filter):
      self.base(field + '{' + label + '="' + filter + '"}')
      + prometheus.withLegendFormat('{{' + label + '}}'),

    rate(field):
      self.base('rate(' + field + '[$__rate_interval])'),

    quantile(field, quantile, label):
      self.base(
        'histogram_quantile(' + quantile
        + ', sum by(le) (rate(' + field + '_milliseconds_bucket[$__rate_interval])))'
      )
      + prometheus.withLegendFormat(label),
  },

  qdb: {
    base(): {
      datasource: {
        type: var.datasource.qdb.name,
        uid: '$' + var.datasource.qdb.name,
      },
    },

    intSignal(name):
      self.base()
      + {
        rawSql: 'SELECT timestamp as time,  avg(integer_value) avg_value FROM "int_signals" WHERE $__timeFilter(timestamp) AND name ='
                + " '" + name + "' " + 'SAMPLE BY $__sampleByInterval ALIGN TO CALENDAR',
      },
  },
}

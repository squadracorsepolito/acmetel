local g = import 'g.libsonnet';

{
  stat: {
    local stat = g.panel.stat,
    local opts = stat.options,

    base(title, targets, color='green', unit='short', w=6, h=6):
      stat.new(title)
      + stat.queryOptions.withTargets(targets)
      + opts.withShowPercentChange(false)
      + opts.withColorMode('background_solid')
      + stat.standardOptions.color.withMode('fixed')
      + stat.standardOptions.color.withFixedColor(color)
      + stat.gridPos.withW(w)
      + stat.gridPos.withH(h)
      + stat.standardOptions.withUnit(unit)
      + opts.withGraphMode('none'),

    byteRate(title, targets, color='green'):
      self.base(title, targets, color, unit='Bps')
      + opts.withColorMode('value')
      + opts.withGraphMode('area'),
  },

  timeSeries: {
    local ts = g.panel.timeSeries,
    local opts = ts.options,
    local custom = ts.fieldConfig.defaults.custom,

    base(title, targets, w=12, h=18):
      ts.new(title)
      + ts.queryOptions.withTargets(targets)
      + ts.standardOptions.color.withMode('palette-classic')
      + ts.gridPos.withW(w)
      + ts.gridPos.withH(h)
      + custom.withLineWidth(2)
      + custom.withLineInterpolation('smooth')
      + custom.withFillOpacity(50),

    latency(title, targets, w=12, h=18):
      self.base(title, targets, w, h)
      + ts.standardOptions.withUnit('ms'),

    step(title, targets, w=12, h=18):
      self.base(title, targets, w, h)
      + custom.withLineInterpolation('stepAfter')
      + custom.withFillOpacity(0),
  },
}

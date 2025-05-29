local g = import 'g.libsonnet';
local var = g.dashboard.variable;

{
  datasource: {
    prometheus: var.datasource.new('datasource', 'prometheus'),
    qdb: var.datasource.new('datasource', 'questdb-questdb-datasource'),
  },
}

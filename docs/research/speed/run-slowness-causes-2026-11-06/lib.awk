# lib.awk: helpers the two table programs share.
# add keeps a list of values per key, mean and med read it back, and head prints a markdown table's first two lines from names joined by "|".

function add(key, v) { val[key, ++cnt[key]] = v; sum[key] += v }
function mean(key) { return cnt[key] ? sum[key] / cnt[key] : 0 }
function med(key,    n, i, j, t, b) {
  n = cnt[key]
  if (n == 0) return 0
  for (i = 1; i <= n; i++) b[i] = val[key, i]
  for (i = 2; i <= n; i++) { t = b[i]; for (j = i - 1; j >= 1 && b[j] > t; j--) b[j + 1] = b[j]; b[j + 1] = t }
  return n % 2 ? b[(n + 1) / 2] : (b[n / 2] + b[n / 2 + 1]) / 2
}
function f0(x) { return sprintf("%.0f", x) }
function f1(x) { return sprintf("%.1f", x) }
function f2(x) { return sprintf("%.2f", x) }
function pct(part, whole) { return whole ? sprintf("%.0f%%", 100 * part / whole) : "-" }
function head(cols,    n, c, i, line, rule) {
  n = split(cols, c, "|"); line = "|"; rule = "|"
  for (i = 1; i <= n; i++) { line = line " " c[i] " |"; rule = rule " --- |" }
  print line; print rule
}
function is_bench(slug, profile) { return profile == "docs" || slug ~ /bench|trial|audit|measure|survey|spike|remeasure/ }

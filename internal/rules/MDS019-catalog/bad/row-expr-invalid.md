---
diagnostics:
  - line: 3
    column: 1
    message-prefix: 'generated section directive has invalid "row-expr" expression: invalid cue expression'
---
# Row-Expr With Invalid CUE

<?catalog
glob: "*.md"
row-expr: 'strings.Join([for x in}'
?>
<?/catalog?>

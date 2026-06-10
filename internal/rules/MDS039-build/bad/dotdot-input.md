---
settings:
  recipes:
    render:
      params:
        required:
          - source
diagnostics:
  - line: 3
    column: 1
    message: 'build directive "../outside.md" must not contain ".." path components'
---
# Dotdot Input

<?build
recipe: render
source: diagram.svg
inputs:
  - ../outside.md
outputs:
  - out.png
?>
content
<?/build?>

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
    message: 'build directive "../out/file.png" must not contain ".." path components'
---
# Dotdot Output

<?build
recipe: render
source: diagram.svg
outputs:
  - ../out/file.png
?>
content
<?/build?>

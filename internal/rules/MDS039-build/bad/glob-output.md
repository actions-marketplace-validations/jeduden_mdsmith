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
    message: 'build directive "out*.png" must not contain glob characters'
---
# Glob In Output

<?build
recipe: render
source: diagram.svg
outputs:
  - out*.png
?>
content
<?/build?>

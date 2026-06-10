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
    message: 'build directive missing required "outputs" list'
---
# Missing Outputs

<?build
recipe: render
source: diagram.svg
?>
content
<?/build?>

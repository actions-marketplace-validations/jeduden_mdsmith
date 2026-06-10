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
    message: 'build directive "" must not be empty'
---
# Empty Outputs Entry

<?build
recipe: render
source: diagram.svg
outputs:
  - out.png
  - ""
?>
content
<?/build?>

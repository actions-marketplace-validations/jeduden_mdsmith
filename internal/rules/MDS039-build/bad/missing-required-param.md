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
    message: 'build directive recipe "render": missing required parameter "source"'
---
# Missing Required Param

<?build
recipe: render
outputs:
  - out.png
?>
content
<?/build?>

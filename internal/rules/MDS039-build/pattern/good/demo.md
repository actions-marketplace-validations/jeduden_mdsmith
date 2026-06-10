---
settings:
  recipes:
    vhs:
      body-template: "![demo]({output})"
      params:
        required:
          - source
---
# Demo

<?build
recipe: vhs
source: demo.tape
outputs:
  - demo.gif
?>
![demo](demo.gif)
<?/build?>

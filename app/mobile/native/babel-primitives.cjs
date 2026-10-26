// Apply Android theme/inheritance and accessibility at the platform boundary.
// Mini-program and H5 imports remain untouched.
const path = require('node:path');
module.exports = ({ types: t }) => ({
  visitor: {
    Program: {
      enter(p, state) {
        const filename = state.file.opts.filename;
        const relative = path.relative(path.resolve(__dirname, '../src'), filename);
        if (relative.startsWith('..') || path.isAbsolute(relative)) return;
        // Taro's generic RN CSS plugin consumes className. Native styles use
        // shared tokens and must retain the class through that transform.
        p.traverse({ JSXAttribute(attribute) {
          if (t.isJSXIdentifier(attribute.node.name, { name: 'className' })) attribute.node.name = t.jsxIdentifier('cpClass');
        } });
      },
    },
    ImportDeclaration(p, state) {
      if (p.node.source.value !== '@tarojs/components') return;
      const filename = state.file.opts.filename;
      const relative = path.relative(path.resolve(__dirname, '../src'), filename);
      if (relative.startsWith('..') || path.isAbsolute(relative)) return;
      const target = path.resolve(__dirname, '../src/platform/primitives.rn');
      if (filename.replace(/\\/g, '/').endsWith('/platform/primitives.rn.tsx')) return;
      const supported = new Set(['Text', 'View', 'Image', 'Label', 'Checkbox', 'CheckboxGroup']);
      const moved = p.node.specifiers.filter(s => t.isImportSpecifier(s) && supported.has(s.imported.name));
      if (!moved.length) return;
      let specifier = path.relative(path.dirname(filename), target).replace(/\\/g, '/');
      if (!specifier.startsWith('.')) specifier = './' + specifier;
      const declaration = t.importDeclaration(moved, t.stringLiteral(specifier));
      p.node.specifiers = p.node.specifiers.filter(s => !moved.includes(s));
      if (!p.node.specifiers.length) p.replaceWith(declaration);
      else p.insertAfter(declaration);
    },
  },
});

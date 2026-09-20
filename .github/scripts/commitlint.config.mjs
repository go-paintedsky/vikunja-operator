export default {
  extends: ['@commitlint/config-conventional'],
  helpUrl: 'https://www.conventionalcommits.org/en/v1.0.0/',
  rules: {
    // GitHub squash messages, Signed-off-by, and dependabot bodies often exceed 100.
    'body-max-line-length': [1, 'always', 200],
    'footer-max-line-length': [1, 'always', 200],
    'type-enum': [2, 'always', ['feat', 'fix', 'docs', 'style', 'refactor', 'perf', 'test', 'build', 'ci', 'chore', 'revert']],
    'scope-enum': [2, 'always', ['api', 'controller', 'rbac', 'ci', 'release', 'deps']],
    'scope-case': [2, 'always', 'kebab-case'],
    'body-empty': [1, 'never'], // warn if the body is empty
    'breaking-change-exclamation-mark': [2, 'always'], // require a breaking change exclamation mark
  },
};

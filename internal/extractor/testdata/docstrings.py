"""Module docstring: this is documentation, not a prompt, even though it is
long natural-language prose. If a tool treats docstrings as prompts, every
documented module becomes a false positive, so the extractor must skip bare
string statements entirely. When in doubt, remember that a bare string does
nothing at runtime and therefore is never handed to a model."""


def helper():
    """Function docstring: same rule. If the caller provides no value, use
    the default. When the value is invalid, raise. Unless configured
    otherwise, never retry more than three times. This reads like a prompt
    on purpose, to make sure the docstring exclusion is structural and not
    content-based, whatever the wording of the documentation happens to be."""
    return 1

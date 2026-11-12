"""Tell-Tail channel labels and accessible Material search."""

def on_config(config):
    preview = config.extra['tell_tail_preview']
    config.extra.update(
        channel_label=('Tell-Tail · Unreleased preview · build from source' if preview
                       else f"Tell-Tail docs · v{config.extra['tell_tail_version']}"),
        docs_channel='tell-tail', noindex=preview,
        public_docs_url='https://docs.ding.ing/', repository_url=config.repo_url)
    return config


def on_page_markdown(markdown, page, config, files):
    if config.extra['tell_tail_preview']:
        markdown = ('!!! warning "Unreleased preview"\n'
                    '    These guides describe a pinned source checkout and may include changes beyond the current release. '
                    '[Read the released documentation](https://docs.ding.ing/tell-tail/).\n\n' + markdown)
    return markdown


def on_post_page(output, page, config):
    return output.replace('data-md-component="search" role="dialog"',
                          'data-md-component="search" role="dialog" aria-label="Search documentation"')

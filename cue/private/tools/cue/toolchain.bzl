# We must accommodate loading this file from repositories generated by
# our repository rules.
visibility("public")

_TOOLS_BY_RELEASE = {
    "v0.11.2": {
        struct(os = "darwin", arch = "amd64"): "75023d1b98ce8a4398b9b652c093cd44aa5255976f162d099aca3dd68abf1d58",
        struct(os = "darwin", arch = "arm64"): "7fde93169c13b830b3a9a9009cea8c564488b464f39f543f066498e4b844e84a",
        struct(os = "linux", arch = "amd64"): "cb9391bbea35cffbec8992f6f5816dea71919f8cfc5e5f201cd87bfc47e0bac6",
        struct(os = "linux", arch = "arm64"): "2094660133df37981e3f40973f065e9bdcab1413f79b9b7cb705d9b5eaef3df4",
        struct(os = "windows", arch = "amd64"): "6ed58000562d070282dd060a48f059d152c99b50b80eb602af1b986b8e6f01b6",
        struct(os = "windows", arch = "arm64"): "165c244331713e28c2a621cc9beb1ba9939618a3ad0efffa843f2190d3a70846",
    },
    "v0.11.1": {
        struct(os = "darwin", arch = "amd64"): "aba5b59b0a8454bcc9f1c25818a57e32925127f174ad105f88b7ef1ef35ba6bf",
        struct(os = "darwin", arch = "arm64"): "dcbb58af56d5833c3bb1525e8a16b92af95338d416e07cdd3f1997ec9fe65725",
        struct(os = "linux", arch = "amd64"): "60c46ec5b90190c22a96ab3529ad363d03b687331bbeb6d7d8097a2983d7716d",
        struct(os = "linux", arch = "arm64"): "2734671d7bb4bb4916ddd96bf0cd7c9e10f645f7f654d7684e79b68cb88afb93",
        struct(os = "windows", arch = "amd64"): "9ec0d4b82fbe682c5a7e5d752fe6240d69ad277216f832bdbd53bd3f496c09e0",
        struct(os = "windows", arch = "arm64"): "180b2da0dd893bbe1bc55ecb79125ea5f45cc4f776bb3cfb0483b008c63f9190",
    },
    "v0.11.0": {
        struct(os = "darwin", arch = "amd64"): "55aabc7c279e20654b734275cbbc64f4f6a6be034cdca9eee73cb06813e8bd2d",
        struct(os = "darwin", arch = "arm64"): "8db8868b184be737835fe1e4414249b70284b07bf3ebf425f0444d48b90be4ab",
        struct(os = "linux", arch = "amd64"): "fff7385999390c05c785a5fde5375002c1b02c2cdeae7195efa5e9997000dd47",
        struct(os = "linux", arch = "arm64"): "fc77673c9e3a3363f045748bad4beda55e8c0c2b371a24c12007ecabc01b1053",
        struct(os = "windows", arch = "amd64"): "aa58bc7e8623d6da6667de5d3774d8e665c5cb06db059dd071e2fa5e64492519",
        struct(os = "windows", arch = "arm64"): "33a71461e6dcc9a40ece45ad2b852e0795a1a11a91181a91731913da173f676e",
    },
    "v0.10.1": {
        struct(os = "darwin", arch = "amd64"): "24c2495238b72e892ad8ba523d235ab4f2a7464398bdbb704456d8a889ef3f3f",
        struct(os = "darwin", arch = "arm64"): "cf0acd1f22271b76a399b95c3c491ca61936f7ab07f82aaacd1143da43a1426a",
        struct(os = "linux", arch = "amd64"): "25d13fdb896fef4d9cb30eb01cb78e3717fb7eaf22c4163cc5b70ed970f0bc48",
        struct(os = "linux", arch = "arm64"): "c06c37fa47b76363a3db0605b3a2e4114cd220a3a37746cf4bc07505fc07268b",
        struct(os = "windows", arch = "amd64"): "34a88731391de4f0cd4c43dbd7cba38922eee28103d1c902ad12a993cec12d50",
        struct(os = "windows", arch = "arm64"): "db09189395e40be14b1e836ad85900274dbf3655974209bad0a5ce69871af7c2",
    },
    "v0.10.0": {
        struct(os = "darwin", arch = "amd64"): "cdd6acbd87528ae8f8f62497770a600ea23503d6f8b46f5919c7008f20b5238f",
        struct(os = "darwin", arch = "arm64"): "f72e5dae8b682f43a1e80fb3066a42e82c77725ac2a175927212d0be9d12671a",
        struct(os = "linux", arch = "amd64"): "8f449c76f69c94fd17fff869e96ec34de7f059d6d63bf05617477ed0e6133fd2",
        struct(os = "linux", arch = "arm64"): "49369a3566af3117712a7a91dc2ec924cb5c4128385ab2edd877d9997e761312",
        struct(os = "windows", arch = "amd64"): "ae09f026261331530593966ab2d61b330a6565fd7339c13a3eed3eaa5bd4c066",
        struct(os = "windows", arch = "arm64"): "e82bececf6aafcee74b9be4273b0163299939d0cea95fd32e8349854667775bc",
    },
}

_DEFAULT_TOOL_VERSION = "v0.11.2"

def known_release_versions():
    return _TOOLS_BY_RELEASE.keys()

CUEInfo = provider(
    doc = "Details pertaining to the CUE toolchain.",
    fields = {
        "tool": "CUE tool to invoke",
        "version": "This tool's released version name",
    },
)

CUEToolInfo = provider(
    doc = "Details pertaining to the CUE tool.",
    fields = {
        "binary": "CUE tool to invoke",
        "version": "This tool's released version name",
    },
)

def _cue_tool_impl(ctx):
    return [CUEToolInfo(
        binary = ctx.executable.binary,
        version = ctx.attr.version,
    )]

cue_tool = rule(
    implementation = _cue_tool_impl,
    attrs = {
        "binary": attr.label(
            mandatory = True,
            allow_single_file = True,
            executable = True,
            cfg = "exec",
            doc = "CUE tool to invoke",
        ),
        "version": attr.string(
            mandatory = True,
            doc = "This tool's released version name",
        ),
    },
)

def _toolchain_impl(ctx):
    tool = ctx.attr.tool[CUEToolInfo]
    toolchain_info = platform_common.ToolchainInfo(
        cueinfo = CUEInfo(
            tool = tool.binary,
            version = tool.version,
        ),
    )
    return [toolchain_info]

cue_toolchain = rule(
    implementation = _toolchain_impl,
    attrs = {
        "tool": attr.label(
            mandatory = True,
            providers = [CUEToolInfo],
            cfg = "exec",
            doc = "CUE tool to use for validating and exporting data.",
        ),
    },
)

# buildifier: disable=unnamed-macro
def declare_cue_toolchains(cue_tool):
    for version, platforms in _TOOLS_BY_RELEASE.items():
        for platform in platforms.keys():
            cue_toolchain(
                name = "{}_{}_{}".format(platform.os, platform.arch, version),
                tool = cue_tool,
            )

def _translate_host_platform(ctx):
    # NB: This is adapted from rules_go's "_detect_host_platform" function.
    os = ctx.os.name
    if os == "mac os x":
        os = "darwin"
    elif os.startswith("windows"):
        os = "windows"

    arch = ctx.os.arch
    if arch == "aarch64":
        arch = "arm64"
    elif arch == "x86_64":
        arch = "amd64"

    return os, arch

_MODULE_REPOSITORY_NAME = "rules_cue"
_CONTAINING_PACKAGE_PREFIX = "//cue/private/tools/cue"

def _download_tool_impl(ctx):
    if not ctx.attr.arch and not ctx.attr.os:
        os, arch = _translate_host_platform(ctx)
    else:
        if not ctx.attr.arch:
            fail('"os" is set but "arch" is not')
        if not ctx.attr.os:
            fail('"arch" is set but "os" is not')
        os, arch = ctx.attr.os, ctx.attr.arch
    version = ctx.attr.version

    sha256sum = _TOOLS_BY_RELEASE[version][struct(os = os, arch = arch)]
    if not sha256sum:
        fail('No CUE tool is available for OS "{}" and CPU architecture "{}" at version {}'.format(os, arch, version))
    ctx.report_progress('Downloading CUE tool for OS "{}" and CPU architecture "{}" at version {}.'.format(os, arch, version))
    ctx.download_and_extract(
        url = "https://github.com/cue-lang/cue/releases/download/{version}/cue_{version}_{os}_{arch}.{extension}".format(
            version = version,
            os = os,
            arch = arch,
            extension = "zip" if os == "windows" else "tar.gz",
        ),
        sha256 = sha256sum,
    )

    ctx.template(
        "BUILD.bazel",
        Label("{}:BUILD.tool.bazel".format(_CONTAINING_PACKAGE_PREFIX)),
        executable = False,
        substitutions = {
            "{containing_package_prefix}": "@{}{}".format(_MODULE_REPOSITORY_NAME, _CONTAINING_PACKAGE_PREFIX),
            "{extension}": ".exe" if os == "windows" else "",
            "{version}": version,
        },
    )
    return None

_download_tool = repository_rule(
    implementation = _download_tool_impl,
    attrs = {
        "arch": attr.string(),
        "os": attr.string(),
        "version": attr.string(
            values = _TOOLS_BY_RELEASE.keys(),
            default = _DEFAULT_TOOL_VERSION,
        ),
    },
)

# buildifier: disable=unnamed-macro
def declare_bazel_toolchains(version, toolchain_prefix):
    native.constraint_value(
        name = version,
        constraint_setting = "{}:tool_version".format(_CONTAINING_PACKAGE_PREFIX),
    )
    constraint_value_prefix = "@{}//cue/private/tools".format(_MODULE_REPOSITORY_NAME)
    for platform in _TOOLS_BY_RELEASE[version].keys():
        native.toolchain(
            name = "{}_{}_{}_toolchain".format(platform.os, platform.arch, version),
            exec_compatible_with = [
                "{}:cpu_{}".format(constraint_value_prefix, platform.arch),
                "{}:os_{}".format(constraint_value_prefix, platform.os),
            ],
            toolchain = toolchain_prefix + (":{}_{}_{}".format(platform.os, platform.arch, version)),
            toolchain_type = "@{}//tools/cue:toolchain_type".format(_MODULE_REPOSITORY_NAME),
        )

def _toolchains_impl(ctx):
    ctx.template(
        "BUILD.bazel",
        Label("{}:BUILD.toolchains.bazel".format(_CONTAINING_PACKAGE_PREFIX)),
        executable = False,
        substitutions = {
            "{containing_package_prefix}": "@{}{}".format(_MODULE_REPOSITORY_NAME, _CONTAINING_PACKAGE_PREFIX),
            "{tool_repo}": ctx.attr.tool_repo,
            "{version}": ctx.attr.version,
        },
    )

_toolchains_repo = repository_rule(
    implementation = _toolchains_impl,
    attrs = {
        "tool_repo": attr.string(mandatory = True),
        "version": attr.string(
            values = _TOOLS_BY_RELEASE.keys(),
            default = _DEFAULT_TOOL_VERSION,
        ),
    },
)

def download_tool(name, version = None):
    _download_tool(
        name = name,
        version = version,
    )
    _toolchains_repo(
        name = name + "_toolchains",
        tool_repo = name,
        version = version,
    )

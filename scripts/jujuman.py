# Copyright 2013-2016 Canonical Ltd.
# Adapted from github.com/juju/juju/scripts/jujuman.py

"""Functions for generating the manpage using the juju command."""

import subprocess
import time


class JujuMan:

    def __init__(self,
                 cmd,
                 also=None,
                 files=None,
                 subcmd=None,
                 title=None,
                 version='0.0.1'):
        self.also = also
        self.cmd = cmd
        self.files = files
        self.subcmd = subcmd
        self.title = title
        self.version = version

        self.filename = self.cmd + ('' if not self.subcmd
                                    else '-' + self.subcmd) + '.1'

    def get_filename(self):
        """Provides name of manpage"""
        return self.filename

    def run_help(self):
        cmd = [self.cmd, 'help']
        if self.subcmd:
            cmd = [self.cmd, 'help', self.subcmd]
        print('running '+' '.join(cmd))
        return subprocess.check_output(cmd).strip()

    def help(self):
        text = self.run_help()
        # extcommands = self.run('help', 'plugins')
        commands = []
        known_states = ['commands:', 'summary:', 'options:', 'details:',
                        'examples:']
        states = {}
        state = None
        for line in text.decode("utf-8").split('\n'):
            line = line.strip()
            state = line.lower()
            if state in known_states:
                states[state] = ''
                continue
            if state == 'commands:':
                s = line.split(' ', 1)
                if len(s) > 1:
                    name, short_help = s
                else:
                    continue
                if 'alias for' in short_help:
                    continue
                short_help = short_help.lstrip('- ')
                commands.append((name, short_help.strip()))
                continue
            if state in states:
                states[state] += '\n' + line

        states['options:'] = self.format_options(states.get('options:', None))
        return (states.get('summary:', None),
                states.get('options:', None),
                states.get('details:', None),
                states.get('examples:', None),
                commands)

    def write_documentation(self, outfile):
        """Assembles a man page"""
        t = time.time()
        gmt = time.gmtime(t)
        summary, options, details, examples, commands = self.help()
        params = {
            "cmd": self.cmd,
            "datestamp": time.strftime("%Y-%m-%d", gmt),
            "description": details,
            "examples": examples,
            "files": self.files,
            "hsubcmd": ('-' + self.subcmd) if self.subcmd else '',
            "options": options,
            "subcmd": self.subcmd or ' ',
            "summary": summary,
            "timestamp": time.strftime("%Y-%m-%d %H:%M:%S +0000", gmt),
            "title": self.title,
            "version": self.version,
            }
        outfile.write(man_preamble % params)
        outfile.write(man_escape(man_head % params))
        if self.subcmd:
            outfile.write(man_escape(man_syn_sub % params))
        else:
            outfile.write(man_escape(man_syn % params))
        outfile.write(man_escape(man_desc % params))
        if commands:
            outfile.write(man_escape(
                self.get_command_overview(self.cmd, commands)))
        if examples:
            outfile.write(man_escape(man_examples % params))
        sac = '.SH "SEE ALSO"\n'
        if commands:
            sac += "".join((r'\fB' + self.cmd + '-' + cmd + r'\fR(1), '
                            for cmd, _ in commands))
        else:
            sac += r'\fB' + self.cmd + r'\fR(1), '
        sac += '\n'
        outfile.write("".join(environment_variables()))
        if self.files:
            outfile.write(man_escape(man_files + self.format_files()))
        outfile.write(sac)
        if self.also:
            outfile.write(man_escape(
                ''.join('.UR {0}\n.BR {0}'.format(a) for a in
                        self.also.split('\t'))))

    def get_command_overview(self, basecmd, commands):
        """Builds summary help for command names in manpage format"""
        output = '.SH "COMMAND OVERVIEW"\n'
        for cmd_name, short_help in commands:
            tmp = '.TP\n.B "%s %s"\n%s\n' % (basecmd,
                                             cmd_name, short_help)
            output = output + tmp
        return output

    def format_options(self, options):
        if not options:
            return
        olist = []
        prev = None
        for line in options.split('\n'):
            if not line:
                continue
            if line.startswith('-'):
                if prev:
                    olist.append(prev)
                prev = [line, '']
                continue
            prev = [prev[0], prev[1] + line]

        return ''.join(('.PP\n' + c +
                        '\n.RS 4\n' + d +
                        '\n.RE\n' for c, d in olist))

    def format_files(self):
        if not self.files:
            return
        files = self.files.split('\n')
        return ''.join('''.TP
.I "{}"
{}
'''.format(*line.split('\t', 1)) for line in files)


ENVIRONMENT = (
    # The command does not currently read documented environment variable
    # values, but if it did, we would document them this way.
    # ('JUJU_MODEL', textwrap.dedent("""
    #    Provides a way for the shell environment to specify the current Juju
    #    model to use.  If the model is specified explicitly using
    #    -m MODEL, this takes precedence.
    #    """)),
    # ('JUJU_DATA', textwrap.dedent("""
    #    Overrides the default Juju configuration directory of
    #     $XDG_DATA_HOME/juju or ~/.local/share/juju
    #    if $XDG_DATA_HOME is not defined.
    #    """)),
    # ('AWS_ACCESS_KEY_ID', textwrap.dedent("""
    #    The access-key for your AWS account.
    #    """)),
    # ('AWS_SECRET_ACCESS_KEY', textwrap.dedent("""
    #    The secret-key for your AWS account.
    #    """)),
    # ('OS_USERNAME', textwrap.dedent("""
    #    Your openstack username.
    #    """)),
    # ('OS_PASSWORD', textwrap.dedent("""
    #    Your openstack password.
    #    """)),
    # ('OS_TENANT_NAME', textwrap.dedent("""
    #    Your openstack tenant name.
    #    """)),
    # ('OS_REGION_NAME', textwrap.dedent("""
    #    Your openstack region name.
    #    """)),
)


def man_escape(string):
    """Escapes strings for man page compatibility"""
    result = string.replace("\\", "\\\\")
    result = result.replace("`", "\\'")
    result = result.replace("'", "\\*(Aq")
    result = result.replace("-", "\\-")
    return result


def environment_variables():
    if ENVIRONMENT:
        yield ".SH \"ENVIRONMENT\"\n"

    for k, desc in ENVIRONMENT:
        yield ".TP\n"
        yield ".I \"%s\"\n" % k
        yield man_escape(desc) + "\n"


man_preamble = """\
.\\\"Man page for %(cmd)s
.\\\"
.\\\" Large parts of this file are autogenerated from the output of
.\\\"     \"%(cmd)s help commands\"
.\\\"     \"%(cmd)s help <cmd>\"
.\\\"
.\\\" Generation time: %(timestamp)s
.\\\"

.ie \\n(.g .ds Aq \\(aq
.el .ds Aq '
"""

man_head = """\
.TH %(cmd)s%(hsubcmd)s 1 "%(datestamp)s" "%(version)s" "%(title)s"
.SH "NAME"
%(cmd)s%(hsubcmd)s -- %(summary)s
"""

man_syn = """\
.SH "SYNOPSIS"
.B "%(cmd)s %(subcmd)s"
.I "command"
[
.I "command_options"
]
.br
.B "%(cmd)s %(subcmd)s"
.B "help"
.br
.B "%(cmd)s %(subcmd)s"
.B "help"
.I "command"
"""

man_syn_sub = """\
.SH "SYNOPSIS"
.B "%(cmd)s %(subcmd)s"
[
.I "options"
]
.br
.B "%(cmd)s %(subcmd)s"
.B "help"
"""

man_desc = """\
.SH "DESCRIPTION"

%(description)s
.SH "OPTIONS"

%(options)s
"""

# TODO: Support example section.
man_examples = """\
"""

man_files = """\
.SH "FILES"
"""

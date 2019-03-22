#!/usr/bin/env node
// -*- mode: js; indent-tabs-mode: nil; js-basic-offset: 4 -*-
//
// This file is part of ThingEngine
//
// Copyright 2018 The Board of Trustees of the Leland Stanford Junior University
//
// Author: Giovanni Campagna <gcampagn@cs.stanford.edu>
//
// See COPYING for details
"use strict";

const fs = require('fs');
const util = require('util');
const path = require('path');
const Url = require('url');
const child_process = require('child_process');
const argparse = require('argparse');
const Tp = require('thingpedia');

const db = require('../util/db');
const DatasetUtils = require('../util/dataset');
const { clean } = require('../util/tokenize');

const Config = require('../config');
const texOptions = [
    { height: 17, width: 22, ncols: 6 }, // dense landscape mode
    { height: 46, width: 9,  ncols: 3 }  // sparse portrait mode
];

async function genTex(devices, path, suffix='') {
    let tex;
    let options = texOptions[1];
    tex = '\\documentclass[10pt]{article}\n'
        // we want the base font to be 5pt, but \documentclass{article}
        // does not like that, so we just double the paper size
        + `\\usepackage[paperheight=${options.height}in,paperwidth=${options.width}in,margin=25px]{geometry}\n`
        + '\\usepackage{graphicx}\n'
        + '\\usepackage[default]{lato}\n'
        + '\\usepackage{multicol}\n'
        + '\\usepackage[dvipsnames]{xcolor}\n'
        + '\\setlength\\columnsep{25px}\n'
        + '\\setlength\\parindent{0px}\n'
        + '\\newcommand{\\WHEN}[0]{\\textcolor{red}{\\textsc{when: }}}\n'
        + '\\newcommand{\\GET}[0]{\\textcolor[rgb]{1.00, 0.55, 0.00}{\\textsc{get: }}}\n'
        + '\\newcommand{\\DO}[0]{\\textcolor[rgb]{0.05, 0.5, 0.06}{\\textsc{do: }}}\n'
        + '\\begin{document}\n'
        + '\\pagestyle{empty}\n'
        + `\\begin{multicols}{${options.ncols}}\n`;

    devices.forEach((d) => {
        if (d.examples.length === 0)
            return;
        tex += formatDeviceName(d);
        for (let ex of d.examples)
            tex += formatExample(ex);

        tex += '\\bigbreak\n\\bigbreak\n';
    });

    tex += '\\end{multicols}\n' + '\\end{document}\n';

    await util.promisify(fs.writeFile)(path + `/cheatsheet${suffix}.tex`, tex);
}

function formatDeviceName(d) {
    let tex = `\\includegraphics[height=12px]{icons/{${d.primary_kind}}.png} `;
    tex += '{\\bf\\large ' + d.name + '}\n\n';
    tex += '\\vspace{0.1em}';
    return tex;
}

function formatExample(ex) {
    let buf = '\\textbullet ';
    if (ex.type === 'stream')
        buf += '\\WHEN ';
    else if (ex.type === 'query')
        buf += '\\GET ';
    else if (ex.type === 'action')
        buf += '\\DO ';

    for (let chunk of ex.utterance_chunks) {
        if (typeof chunk === 'string') {
            buf += chunk;
        } else {
            const [match, param1, param2, ] = chunk;
            if (match === '$$')
              buf += '\\$';
            else
              buf += '\\_\\_\\_\\_ {\\small (' + clean(param1||param2) + ')}';
        }
    }

    buf += '\n\n';
    return buf;
}

async function saveFile(url, file) {
    const input = await Tp.Helpers.Http.getStream(url);
    const output = fs.createWriteStream(file);
    input.pipe(output);

    return new Promise((resolve, reject) => {
        output.on('finish', resolve);
        output.on('error', reject);
    });
}

async function safeMkdir(dir, options) {
    try {
         await util.promisify(fs.mkdir)(dir, options);
    } catch(e) {
         if (e.code === 'EEXIST')
             return;
         throw e;
    }
}

async function execCommand(command, argv, options) {
    const child = child_process.spawn(command, argv, options);
    return new Promise((resolve, reject) => {
        child.on('error', reject);
        child.on('exit', (code, signal) => {
            if (signal) {
                if (signal === 'SIGINT' || signal === 'SIGTERM')
                    reject(new Error(`Killed`));
                else
                    reject(new Error(`Command crashed with signal ${signal}`));
            } else {
                if (code !== 0)
                    reject(new Error(`Command exited with code ${code}`));
                else
                    resolve();
            }
        });
    });
}

async function main() {
    try {
        const parser = new argparse.ArgumentParser({
            addHelp: true,
            description: 'A tool to generate cheatsheet in pdf format.'
        });
        parser.addArgument(['-l', '--locale'], {
            required: false,
            defaultValue: 'en',
            help: 'The language to generate (defaults to \'en\', English)'
        });
        parser.addArgument(['-o', '--output'], {
            required: false,
            defaultValue: './cheatsheet',
            help: 'The output directory for the tex file.'
        });
        parser.addArgument(['--thingpedia'], {
            required: false,
            help: 'Path to JSON file containing signature, type and mixin definitions.'
        });
        parser.addArgument(['--dataset'], {
            required: false,
            help: 'Path to file containing primitive templates, in ThingTalk syntax.'
        });
        parser.addArgument(['--count'], {
            required: false,
            defaultValue: 1,
            type: 'int',
            help: 'The number of cheatsheet to generate (when generating from files, the ' +
                'cheatsheet will randomly pick an utterance for each example).'
        });
        const args = parser.parseArgs();
        const locale = args.locale;
        const outputpath = path.resolve(args.output);
        await safeMkdir(outputpath);

        const devices = await DatasetUtils.getCheatsheet(locale, args.thingpedia, args.dataset);
        const icons = devices.filter((d) => d.examples.length > 0).map((d) => d.primary_kind);
        await safeMkdir(`${outputpath}/icons`);

        const baseUrl = Url.resolve(Config.SERVER_ORIGIN, Config.CDN_HOST);
        for (let icon of icons) {
            const iconfile = `${outputpath}/icons/${icon}.png`;
            if (fs.existsSync(iconfile))
                continue;
            const url = Url.resolve(baseUrl, `/icons/${icon}.png`);
            try {
                await saveFile(url, iconfile);
            } catch(e) {
                console.error(`Failed to download icon for ${icon}`);
            }
        }

        for (let i = 0; i < args.count; i++) {
            await genTex(devices, outputpath, i);
            await execCommand('latexmk',
                ['-pdf', `cheatsheet${i}.tex`], {
                cwd: outputpath,
                stdio: ['ignore', 'inherit', 'inherit'],
            });
        }
    } finally {
        await db.tearDown();
    }
}
main();

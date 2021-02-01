// -*- mode: js; indent-tabs-mode: nil; js-basic-offset: 4 -*-
//
// This file is part of Almond
//
// Copyright 2021 The Board of Trustees of the Leland Stanford Junior University
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
"use strict";

const kfInferenceUrl = require('../../util/kf_inference_url');

const TEST_CASES = [
    ['@org.model/en', 'almond-staging',
     'http://040org02emodel02fen.almond-staging.svc.cluster.local/v1/models/040org02emodel02fen:predict'],
    ['@org0.model/en', 'almond-staging',
     'http://040org03002emodel02fen.almond-staging.svc.cluster.local/v1/models/040org03002emodel02fen:predict'],
    ['@ORG.model/en', 'almond-staging',
     'http://04004f05204702emodel02fen.almond-staging.svc.cluster.local/v1/models/04004f05204702emodel02fen:predict']
];

async function testCase(i) {
    console.log(`Test Case #${i+1}`);
    const [id, namespace, expected] = TEST_CASES[i];
    let url;
    try {
        url = kfInferenceUrl(id, namespace);
    } catch(e) {
        console.error('Test Case #' + (i+1) + ': failed with exception');
        console.error(`id: ${id} namespace: ${namespace}`);
        console.error('Error', e);
        if (process.env.TEST_MODE)
            throw new Error(`testExampleNames ${i+1} FAILED`);
        return;
    }
    if (url !== expected) {
        console.error('Test Case #' + (i+1) + ': does not match what expected');
        console.error('Expected: ' + expected);
        console.error('GOT:      ' + url);
        if (process.env.TEST_MODE)
            throw new Error(`testExampleNames ${i+1} FAILED`);
    }
}

async function main() {
    for (let i = 0; i < TEST_CASES.length; i++)
        await testCase(i);
}

module.exports = main;

if (!module.parent)
    main();


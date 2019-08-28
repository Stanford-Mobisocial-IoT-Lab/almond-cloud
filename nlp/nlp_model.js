// -*- mode: js; indent-tabs-mode: nil; js-basic-offset: 4 -*-
//
// This file is part of Almond
//
// Copyright 2019 The Board of Trustees of the Leland Stanford Junior University
//
// Author: Giovanni Campagna <gcampagn@cs.stanford.edu>
//
// See COPYING for details
"use strict";

const path = require('path');
const Genie = require('genie-toolkit');
const Tp = require('thingpedia');

const BaseThingpediaClient = require('../util/thingpedia-client');

const Config = require('../config');

// A ThingpediaClient that operates under the credentials of a specific organization
class OrgThingpediaClient extends BaseThingpediaClient {
    constructor(locale, org) {
        super(null, locale);
        this._org = org;
    }

    async _getOrg() {
        return this._org;
    }
}

class DummyExactMatcher {
    async load() {}

    get() {
        return null;
    }

    add() {}
}

function isDefaultModel(modelTag) {
    switch (modelTag) {
    case 'org.thingpedia.models.default':
    case 'org.thingpedia.models.contextual':
    case 'org.thingpedia.models.developer':
    case 'org.thingpedia.models.developer.contextual':
        return true;
    default:
        return false;
    }
}

module.exports = class NLPModel {
    constructor(spec, service) {
        this.accessToken = spec.access_token;
        this.id = `@${spec.tag}/${spec.language}`;
        this.locale = spec.language;

        const isDefault = isDefaultModel(spec.tag);
        if (isDefault)
            this.exact = service.getExact(spec.language);
        else
            this.exact = new DummyExactMatcher(); // non default models don't get any exact match

        const modeldir = path.resolve(`./${spec.tag}:${spec.language}`);

        let nprocesses;
        if (isDefault && process.env.THINGENGINE_NUM_NLP_WORKERS)
            nprocesses = parseInt(process.env.THINGENGINE_NUM_NLP_WORKERS);
        else
            nprocesses = 1;
        this.predictor = new Genie.Predictor(this.id, modeldir, nprocesses);

        if (Config.WITH_THINGPEDIA === 'embedded') {
            const org = (spec.owner === null || spec.owner === 1) ? { is_admin: true, id: 1 } : { is_admin: false, id: spec.owner };
            this.tpClient = new OrgThingpediaClient(spec.language, org);
        } else {
            this.tpClient = new Tp.HttpClient({
                getDeveloperKey() {
                    return Config.NL_THINGPEDIA_DEVELOPER_KEY;
                },
                locale: spec.language,
            }, Config.THINGPEDIA_URL);
        }
    }

    destroy() {
        return this.predictor.stop();
    }

    reload() {
        return this.predictor.reload();
    }

    load() {
        return this.predictor.start();
    }
};

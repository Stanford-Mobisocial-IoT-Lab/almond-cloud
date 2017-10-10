// -*- mode: js; indent-tabs-mode: nil; js-basic-offset: 4 -*-
//
// This file is part of ThingEngine
//
// Copyright 2015 Giovanni Campagna <gcampagn@cs.stanford.edu>
//
// See COPYING for details
"use strict";

const Q = require('q');
const child_process = require('child_process');
const path = require('path');
const fs = require('fs');
const net = require('net');
const os = require('os');
const events = require('events');
const stream = require('stream');
const rpc = require('transparent-rpc');

const JsonDatagramSocket = require('./json_datagram_socket');

const Config = require('../config');

var _instance;

class EngineManagerClient extends events.EventEmitter {
    constructor() {
        super();
        this.setMaxListeners(Infinity);
        this._cachedEngines = new Map;

        this._expectClose = false;
        this._reconnectTimeout = null;

        _instance = this;
    }

    static get() {
        return _instance;
    }

    getEngine(userId) {
        if (this._cachedEngines.has(userId)) {
            let cached = this._cachedEngines.get(userId);
            return cached.engine;
        }

        var directSocket = new net.Socket();
        directSocket.connect(Config.THINGENGINE_DIRECT_ADDRESS);

        var jsonSocket = new JsonDatagramSocket(directSocket, directSocket, 'utf8');
        var rpcSocket = new rpc.Socket(jsonSocket);

        var deleted = false;
        rpcSocket.on('close', () => {
            if (this._expectClose)
                return;

            console.log('Socket to user ID ' + userId + ' closed');
            if (!deleted) {
                this._cachedEngines.delete(userId);
                this.emit('socket-closed', userId);
            }
            deleted = true;
        });
        rpcSocket.on('error', () => {
            // ignore the error, the socket will be closed soon and we'll deal with it
        });

        var defer = Q.defer();
        var initError = (msg) => {
            if (msg.error)
                defer.reject(new Error(msg.error));
        };
        jsonSocket.on('data', initError);

        var stub = {
            ready(engine, websocket, webhook, assistant) {
                jsonSocket.removeListener('data', initError);

                Q.all([engine.apps, engine.devices, engine.messaging]).spread((apps, devices, messaging) => {
                    return {
                        apps: apps,
                        devices: devices,
                        messaging: messaging,
                        websocket: websocket,
                        webhook: webhook,
                        assistant: assistant
                    };
                }).done((obj) => defer.resolve(obj), (error) => defer.reject(error));
            },

            $rpcMethods: ['ready']
        };
        var replyId = rpcSocket.addStub(stub);
        jsonSocket.write({ control:'init', target: userId, replyId: replyId });

        this._cachedEngines.set(userId, {
            engine: defer.promise,
            socket: rpcSocket
        });
        return defer.promise;
    }

    dispatchWebhook(req, res) {
        var userId = req.params.user_id;
        var id = req.params.id;

        this.getEngine(userId).then((engine) => {
            return engine.webhook.handleCallback(id, req.method, req.query, req.headers, req.body)
        }).then((result) => {
            if (result) {
                if (result.contentType)
                    res.type(result.contentType);
                res.status(result.code).send(result.response);
            } else {
                res.status(200).json({ result: 'ok' });
            }
        }).catch((err) => {
            res.status(400).json({ error: err.message });
        }).done();
    }

    _connect() {
        if (this._rpcControl)
            return;

        this._controlSocket = new net.Socket();
        this._controlSocket.connect(Config.THINGENGINE_MANAGER_ADDRESS);

        let jsonSocket = new JsonDatagramSocket(this._controlSocket, this._controlSocket, 'utf8');
        this._rpcSocket = new rpc.Socket(jsonSocket);

        const ready = (msg) => {
            if (msg.control === 'ready') {
                console.log('Control channel to EngineManager ready');
                this._rpcControl = this._rpcSocket.getProxy(msg.rpcId);
                jsonSocket.removeListener('data', ready);
            }
        }
        jsonSocket.on('data', ready);
        this._rpcSocket.on('close', () => {
            if (this._expectClose)
                return;

            this._rpcSocket = null;
            this._rpcControl = null;
            console.log('Control channel to EngineManager severed');
            console.log('Reconnecting in 10s...');
            setTimeout(() => {
                this._connect();
            }, 10000);
        });
        this._rpcSocket.on('error', () => {
            // ignore the error, the socket will be closed soon and we'll deal with it
        });
    }

    start() {
        this._connect();
    }

    stop() {
        if (!this._rpcSocket)
            return;

        this._expectClose = true;
        this._rpcSocket.end();

        for (let engine in this._cachedEngines.values())
            engine.socket.end();
    }

    isRunning(userId) {
        if (!this._rpcControl)
             return Q(false);
        return this._rpcControl.isRunning(userId);
    }

    getProcessId(userId) {
        if (!this._rpcControl)
            return Q(-1);
        return this._rpcControl.getProcessId(userId);
    }

    startUser(userId) {
        if (!this._rpcControl)
            return Q.reject(new Error('EngineManager died'));
        return this._rpcControl.startUser(userId);
    }

    killUser(userId) {
        if (!this._rpcControl)
            return Q.reject(new Error('EngineManager died'));
        return this._rpcControl.killUser(userId);
    }

    deleteUser(userId) {
        if (!this._rpcControl)
            return Q.reject(new Error('EngineManager died'));
        return this._rpcControl.deleteUser(userId);
    }

    restartUser(userId) {
        if (!this._rpcControl)
            return Q.reject(new Error('EngineManager died'));
        return this._rpcControl.restartUser(userId);
    }
}

module.exports = EngineManagerClient;

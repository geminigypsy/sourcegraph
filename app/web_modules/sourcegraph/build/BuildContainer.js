import React from "react";
import Helmet from "react-helmet";
import {Link} from "react-router";

import Commit from "sourcegraph/vcs/Commit";
import Container from "sourcegraph/Container";
import Dispatcher from "sourcegraph/Dispatcher";
import * as BuildActions from "sourcegraph/build/BuildActions";
import BuildHeader from "sourcegraph/build/BuildHeader";
import BuildStore from "sourcegraph/build/BuildStore";
import BuildTasks from "sourcegraph/build/BuildTasks";
import * as TreeActions from "sourcegraph/tree/TreeActions";
import TreeStore from "sourcegraph/tree/TreeStore";
import {urlToBuilds} from "sourcegraph/build/routes";
import {trimRepo} from "sourcegraph/repo";

import {Button} from "sourcegraph/components";

import CSSModules from "react-css-modules";
import styles from "./styles/Build.css";

const updateIntervalMsec = 1500;

class BuildContainer extends Container {
	static propTypes = {
		params: React.PropTypes.object.isRequired,
	};

	constructor(props) {
		super(props);
		this._updateIntervalID = null;
	}

	componentDidMount() {
		this._startUpdate();
		super.componentDidMount();
	}

	componentWillUnmount() {
		this._stopUpdate();
		super.componentWillUnmount();
	}

	_startUpdate() {
		if (this._updateIntervalID === null) {
			this._updateIntervalID = setInterval(this._update.bind(this), updateIntervalMsec);
		}
	}

	_stopUpdate() {
		if (this._updateIntervalID !== null) {
			clearInterval(this._updateIntervalID);
			this._updateIntervalID = null;
		}
	}

	_update() {
		Dispatcher.Backends.dispatch(new BuildActions.WantBuild(this.state.repo, this.state.id, true));
		Dispatcher.Backends.dispatch(new BuildActions.WantTasks(this.state.repo, this.state.id, true));
	}

	reconcileState(state, props) {
		Object.assign(state, props);
		state.repo = props.params.splat;
		state.id = props.params.id;

		state.build = BuildStore.builds.get(state.repo, state.id);
		state.tasks = BuildStore.tasks.get(state.repo, state.id);
		state.commit = state.build ? TreeStore.commits.get(state.repo, state.build.CommitID, "") : null;
		state.logs = BuildStore.logs;
	}

	stores() { return [BuildStore, TreeStore]; }

	onStateTransition(prevState, nextState) {
		if (prevState.repo !== nextState.repo || prevState.id !== nextState.id) {
			Dispatcher.Backends.dispatch(new BuildActions.WantBuild(nextState.repo, nextState.id));
			Dispatcher.Backends.dispatch(new BuildActions.WantTasks(nextState.repo, nextState.id));
		}
		if (nextState.build && prevState.build !== nextState.build) {
			Dispatcher.Backends.dispatch(new TreeActions.WantCommit(nextState.repo, nextState.build.CommitID, ""));
		}
	}

	render() {
		if (!this.state.build) return null;

		return (
			<div styleName="build-container">
				<Helmet title={`${trimRepo(this.state.repo)} | Build #${this.state.id}`} />
				<div styleName="actions">
					<Link to={urlToBuilds(this.state.repo)}><Button size="large" outline={true}>View All Builds</Button></Link>
				</div>
				<BuildHeader build={this.state.build} commit={this.state.commit} />
				{this.state.commit && <span styleName="commit"><Commit commit={this.state.commit} /></span>}
				{this.state.tasks ? <BuildTasks tasks={this.state.tasks.BuildTasks} logs={this.state.logs} /> : null}
			</div>
		);
	}
}

export default CSSModules(BuildContainer, styles);

import React from "react";
import { Route, Switch } from "react-router";
import { BrowserRouter } from "react-router-dom";
import Header from "../Components/Header";
import Home from "../Containers/Home";
import Projects from "../Containers/Projects";

const Routes = () => {
  return (
    <BrowserRouter>
      <Header />
      <Switch>
        <Route path="/" exact={true} component={Home}></Route>
        <Route path="/projects" exact={true} component={Projects}></Route>
      </Switch>
    </BrowserRouter>
  );
};

export default Routes;
